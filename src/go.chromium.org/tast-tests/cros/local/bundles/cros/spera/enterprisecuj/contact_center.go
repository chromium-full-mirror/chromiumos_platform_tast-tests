// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterprisecuj

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	cx "go.chromium.org/tast-tests/cros/local/bundles/cros/spera/enterprisecuj/citrix"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Since the resolution of each DUT may be different, each coordinate file must be recorded separately.
// Now only recording is made for the hatch device. If replaying on other machines, the case may fail due to wrong coordinates.
// TODO(b/243053294): Since these file may change frequently, it's better the test download the files based on a var.
const (
	contactCenterCoordinatesFile = "citrix/contact_center_coordinates.json"
	contactCenterUIWaitFile      = "citrix/contact_center_uiwait.json"
)

// ContactCenterData lists all icon data used in the contact center cuj.
var ContactCenterData = []string{
	// The following are icons used by uidetection in the contact center cuj.
	cx.IconToolbar,
	cx.IconSearch,
	cx.IconDesktop,
	cx.IconTrackerCancel,
	cx.IconChromeTaskManager,
	cx.IconChromeActive,
	cx.IconZoomJoin,
	// The following are files used in replay mode.
	contactCenterCoordinatesFile,
	contactCenterUIWaitFile,
}

// Test scenario for ContactCenter CUJ:
// 1. Open Citrix Workspace app.
// 2. Open 6 browser windows x 2 tabs (Substitute for CC app) inside Citrix workspace.
// 3. Open a document in Adobe pdf reader inside Citrix.
// 4. Drive connected external 4K display, with Citrix client in one display
// 5. Start Zoom app in Citrix and connect to 5p video call.
// 6. Present a window inside Citrix
// 7. Concurrently Open 2 browser windows x 4 tabs (google search, wikipedia, google docs) in CrOS, outside Citrix.
// 8. Take notes at 70wpm in Google docs outside Citrix.
// 9. Concurrently switch between tabs and windows outside Citrix.
// 10. Search for text within a browser window outside Citrix and scroll the window (use wikipedia)
// 11. Concurrently minimize and redisplay all browser windows inside Citrix.

// Run runs scenario for contact center cuj.
func (c *ContactCenterScenario) Run(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, citrix *cx.Citrix, params *TestParams) (retErr error) {
	const noteContent = "Sample note"
	googleDocsEdited := false
	// Substitute Google, Google Photos, Wikipedia, WebMD for health provider website.
	firstChromeURLs := []string{cuj.GoogleURL, cuj.GooglePhotosURL}
	secondChromeURLs := []string{cuj.WikipediaURL, cuj.WebMDURL}
	thirdChromeURLs := []string{cuj.GoogleURL, cuj.WikipediaURL}
	windows := [][]string{firstChromeURLs, secondChromeURLs, thirdChromeURLs}
	windows = append(windows, windows...)
	openWindows := func(ctx context.Context) error {
		for _, chromeURLs := range windows {
			if err := citrix.OpenChromeWithURLs(chromeURLs)(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	firstCrosChromeURLs := []string{cuj.GoogleURL, cuj.WikipediaURL, cuj.WikipediaURL}
	secondCrosChromeURLs := []string{cuj.GoogleURL, cuj.WikipediaURL, cuj.NewGoogleDocsURL}
	crosWindows := [][]string{firstCrosChromeURLs, secondCrosChromeURLs}
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	chromeApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find the Chrome app")
	}
	openCrosBrowserWithTabs := func(ctx context.Context) error {
		for _, urls := range crosWindows {
			for idx, url := range urls {
				conn, err := params.UIHandler.NewChromeTab(ctx, params.Br, url, idx == 0)
				if err != nil {
					return errors.Wrapf(err, "failed to open %s", url)
				}
				// We don't need to keep the connection, so close it before leaving this function.
				defer conn.Close()
				if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
					return errors.Wrap(err, "failed to wait for page to finish loading")
				}
			}
		}
		return nil
	}
	defer func(ctx context.Context) {
		// If fails, dump the last screen before deleting the document.
		faillog.DumpUITreeWithScreenshotOnError(ctx, params.OutDir, func() bool { return retErr != nil }, params.Cr, "ui_dump_last")
		if googleDocsEdited {
			if err := uiauto.Combine("delete Google Docs",
				params.UIHandler.SwitchToAppWindowByName(chromeApp.Name, "Google Docs"),
				googledocs.DeleteDoc(tconn))(ctx); err != nil {
				// Only log the error.
				testing.ContextLog(ctx, "Failed to clean up the document: ", err)
			}
		}
		cuj.CloseAllWindows(ctx, tconn)
	}(cleanUpCtx)

	takeNotesInGoogleDocs := func(ctx context.Context) error {
		const paragraph = "The Little Prince's story follows a young prince who visits various planets in space, " +
			"including Earth, and addresses themes of loneliness, friendship, love, and loss. "
		if err := googledocs.EditDoc(tconn, kb, paragraph)(ctx); err != nil {
			return err
		}
		googleDocsEdited = true
		return nil
	}

	switchCrosBrowserTabs := func(ctx context.Context) error {
		for windowIndex, tabs := range crosWindows {
			if err := params.UIHandler.SwitchToAppWindowByIndex(chromeApp.Name, windowIndex)(ctx); err != nil {
				return err
			}
			for tabIndex := range tabs {
				if err := params.UIHandler.SwitchToChromeTabByIndex(tabIndex)(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	}

	searchWikiAndScroll := func(ctx context.Context) error {
		conn, err := params.UIHandler.NewChromeTab(ctx, params.Br, cuj.WikipediaURL, true)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", cuj.WikipediaURL)
		}
		defer conn.Close()
		waitForWikiPage := func(ctx context.Context) error {
			if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
				return errors.Wrap(err, "failed to wait for wiki page to finish loading")
			}
			return nil
		}

		scrollPage := func(ctx context.Context) error {
			scrollActions := params.UIHandler.ScrollChromePage(ctx)
			for _, scroll := range scrollActions {
				if err := scroll(ctx); err != nil {
					return errors.Wrap(err, "failed to scroll page")
				}
			}
			return err
		}
		return uiauto.Combine("search wiki and scroll",
			waitForWikiPage,
			kb.TypeAction("Call center"),
			kb.AccelAction("Enter"),
			waitForWikiPage,
			scrollPage,
		)(ctx)
	}
	switchToCitrixWorkspace := func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		shelfAppButtonRegex := regexp.MustCompile(ash.ShelfAppButtonClassNameRegex)
		workspaceShelfButton := nodewith.NameContaining("VDA").ClassNameRegex(shelfAppButtonRegex).Role(role.Button)
		nodeInfo, err := ui.Info(ctx, workspaceShelfButton)
		if err != nil {
			return errors.Wrap(err, "failed to get info for workspace shelf button")
		}
		return params.UIHandler.SwitchToAppWindow(nodeInfo.Name)(ctx)
	}
	apps := []cx.WindowsApp{cx.Adobe, cx.GoogleChrome}
	if params.TestMode == cx.ReplayMode {
		if err := citrix.LoadRecordFile(contactCenterCoordinatesFile, contactCenterUIWaitFile); err != nil {
			return err
		}
	}
	return uiauto.NamedCombine("run the contact center cuj scenario",
		citrix.ConnectRemoteDesktop(params.DesktopName),
		citrix.FullscreenDesktop(),
		citrix.CloseApplications(apps),
		citrix.FocusOnDesktop(),
		// 2. Open 6 browser windows x 2 tabs (Substitute for CC app) inside Citrix workspace.
		openWindows,
		// 3. Open a document in Adobe pdf reader inside Citrix.
		citrix.OpenPDF(),
		// 4. Drive connected external 4K display, with Citrix client in one display.
		// TODO:(b/215417608): At present, uidetection doesn't support extended displays.
		// After support, the following line can be uncommented.
		// cuj.SwitchWindowToDisplay(ctx, tconn, kb, true),
		// 5. Start Zoom app in Citrix and connect to 5p video call.
		citrix.SwitchWindow(),
		citrix.OpenZoom(c.zoomInviteLink, c.zoomAccount, c.zoomPassword),
		// 6. Present a window inside Citrix.
		citrix.PresentWindowFromZoom(),
		// 7. Concurrently Open 2 browser windows x 4 tabs (google search, wikipedia, google docs) in CrOS, outside Citrix.
		openCrosBrowserWithTabs,
		// 8. Take notes at 70wpm in Google docs outside Citrix.
		takeNotesInGoogleDocs,
		// 9. Concurrently switch between tabs and windows outside Citrix.
		switchCrosBrowserTabs,
		// 10. Search for text within a browser window outside Citrix and scroll the window (use wikipedia)
		searchWikiAndScroll,
		// 11. Concurrently minimize and redisplay all browser windows inside Citrix.
		switchToCitrixWorkspace,
		citrix.StopSharing(),
		citrix.ShowTheDesktop(),
		citrix.CloseApplications(apps),
		citrix.ExitFullscreenDesktop(),
	)(ctx)
}

// ContactCenterScenario implements the CitrixScenario interface.
type ContactCenterScenario struct {
	zoomInviteLink string
	zoomAccount    string
	zoomPassword   string
}

var _ CitrixScenario = (*ContactCenterScenario)(nil)

// NewContactCenterScenario creates contact center instance which implements CitrixScenario interface.
func NewContactCenterScenario(zoomInviteLink, zoomAccount, zoomPassword string) *ContactCenterScenario {
	return &ContactCenterScenario{
		zoomInviteLink: zoomInviteLink,
		zoomAccount:    zoomAccount,
		zoomPassword:   zoomPassword,
	}
}
