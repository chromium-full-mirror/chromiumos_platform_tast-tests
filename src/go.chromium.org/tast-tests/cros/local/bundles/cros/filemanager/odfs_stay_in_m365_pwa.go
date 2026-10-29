// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/office"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type userParamStayInM365 struct {
	username            string
	password            string
	isConsumerMicrosoft bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           OdfsStayInM365Pwa,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that we stay within the M365 PWA when creating a new file or opening an existing file from M365",
		BugComponent:   "b:1401215", // ChromeOS > Software > Commercial (Enterprise) > Identity > 3P IdP > Enterprise Clippy
		Timeout:        5 * time.Minute,
		Contacts: []string{
			"cros-commercial-clippy-eng@google.com",
			"lmasopust@google.com",
			"emaxx@chromium.org",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier_secondary",
		},
		VarDeps: []string{
			"onedrive.accountPool",
			"onedrive.managedusernamemicrosoft",
			"onedrive.managedpassword",
		},
		Fixture: "onedriveManaged",
		Params: []testing.Param{{
			Name: "consumer",
			Val: userParamStayInM365{
				isConsumerMicrosoft: true,
			},
		}, {
			Name: "commercial",
			Val: userParamStayInM365{
				isConsumerMicrosoft: false,
				username:            "onedrive.managedusernamemicrosoft",
				password:            "onedrive.managedpassword",
			},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.WebAppInstallForceList{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOfficeCloudUpload{}, pci.Served),
		},
	})
}

// oneDriveWindowTitleRegex matches M365 OneDrive PWA window/page titles (e.g.
// "Home - OneDrive", "My files - OneDrive", optionally followed by
// " - <profile>"), while excluding ChromeOS Files SWA
// ("Files - Microsoft OneDrive").
var oneDriveWindowTitleRegex = regexp.MustCompile(`(?i)^(.*- )?OneDrive($| - .*)`)

// closeMicrosoftOneDriveWindow finds the Microsoft OneDrive PWA window and
// closes it.
func closeMicrosoftOneDriveWindow(tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn).WithTimeout(15 * time.Second)
		oneDriveWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameRegex(oneDriveWindowTitleRegex).First()
		oneDriveRoot := nodewith.Role(role.RootWebArea).NameRegex(oneDriveWindowTitleRegex).First()
		webAppToolbar := nodewith.Role(role.Pane).HasClass("WebAppFrameToolbarView").Ancestor(oneDriveWindow)
		closeButton := nodewith.Role(role.Button).Name("Close").HasClass("FrameCaptionButton").Ancestor(oneDriveWindow)

		if err := ui.WaitUntilExists(webAppToolbar)(ctx); err != nil {
			return errors.Wrap(err, "failed to find the MS365 OneDrive window to close")
		}
		if err := ui.LeftClickUntil(closeButton, ui.Gone(oneDriveWindow))(ctx); err != nil {
			return errors.Wrap(err, "failed to close the MS365 OneDrive window")
		}
		if err := ui.WaitUntilGone(oneDriveRoot)(ctx); err != nil {
			return errors.Wrap(err, "failed waiting for the MS365 OneDrive root web area to close")
		}
		return nil
	}
}

// maybeDismissCopilotPrompts dismisses optional prompts (such as the consumer
// "Personalize your ads experience" dialog) that may block the Copilot UI.
func maybeDismissCopilotPrompts(ui *uiauto.Context, m365Context *nodewith.Finder) uiauto.Action {
	adConsentDialog := nodewith.Ancestor(m365Context).Role(role.AlertDialog).NameContaining("Personalize your ads experience").First()
	genericAdsButton := nodewith.Ancestor(m365Context).Role(role.Button).Name("Generic ads").First()

	return func(ctx context.Context) error {
		if err := ui.WaitUntilExists(m365Context)(ctx); err != nil {
			return errors.Wrap(err, "failed to find the Microsoft Copilot context")
		}
		if _, err := ui.WithTimeout(5*time.Second).FindAnyExists(ctx, adConsentDialog, genericAdsButton); err == nil {
			if err := ui.LeftClickUntil(genericAdsButton, ui.Gone(genericAdsButton))(ctx); err != nil {
				return errors.Wrap(err, "failed to dismiss Copilot ads consent prompt")
			}
		}
		return nil
	}
}

// openAppFromCopilotLauncher opens the "Apps and more" ("App Launcher") dialog
// inside Microsoft Copilot and launches appName (e.g. "PowerPoint" or
// "OneDrive"), waiting until expectedWindow exists.
func openAppFromCopilotLauncher(ui *uiauto.Context, m365Context *nodewith.Finder, appName string, expectedWindow *nodewith.Finder) uiauto.Action {
	appsButton := nodewith.Ancestor(m365Context).Role(role.Button).NameRegex(regexp.MustCompile("(?i)^Apps( and more)?$")).First()
	appsToggleButton := nodewith.Ancestor(m365Context).Role(role.ToggleButton).NameRegex(regexp.MustCompile("(?i)^Apps( and more)?$")).First()
	expandSidebarButton := nodewith.Ancestor(m365Context).Role(role.Button).NameRegex(regexp.MustCompile("(?i)^(Open|Expand) (sidebar|navigation).*")).First()
	appMenuItem := nodewith.Ancestor(m365Context).Role(role.MenuItem).NameContaining(appName).First()
	appLink := nodewith.Ancestor(m365Context).Role(role.Link).NameContaining(appName).First()

	return func(ctx context.Context) error {
		var lastErr error
		for attempt := 0; attempt < 3; attempt++ {
			targetAppItem, err := ui.WithTimeout(time.Second).FindAnyExists(ctx, appMenuItem, appLink)
			if err != nil {
				if ui.WithTimeout(2*time.Second).Exists(expandSidebarButton)(ctx) == nil {
					_ = ui.LeftClick(expandSidebarButton)(ctx)
				}
				foundAppsBtn, btnErr := ui.WithTimeout(10*time.Second).FindAnyExists(ctx, appsButton, appsToggleButton)
				if btnErr != nil {
					return errors.Wrap(btnErr, "failed to find Apps button in Microsoft Copilot")
				}
				for openAttempt := 0; openAttempt < 3; openAttempt++ {
					if clickErr := ui.LeftClick(foundAppsBtn)(ctx); clickErr != nil {
						lastErr = clickErr
						continue
					}
					targetAppItem, err = ui.WithTimeout(5*time.Second).FindAnyExists(ctx, appMenuItem, appLink)
					if err == nil {
						break
					}
				}
				if err != nil {
					lastErr = errors.Wrapf(err, "failed to find %s item in App Launcher", appName)
					continue
				}
			}

			if err := ui.LeftClick(targetAppItem)(ctx); err != nil {
				lastErr = errors.Wrapf(err, "failed to click %s in App Launcher", appName)
				continue
			}
			err = ui.WithTimeout(15 * time.Second).WaitUntilExists(expectedWindow)(ctx)
			if err == nil {
				return nil
			}
			lastErr = err
		}
		return errors.Wrapf(lastErr, "failed to launch %s from Microsoft Copilot", appName)
	}
}

// unhideOneDriveAppRoot strips aria-hidden="true" from consumer OneDrive's
// #appRoot and body containers (which Fluent UI / Tabster can leave set,
// pruning the page body from chrome.automation) and attaches a MutationObserver
// so Tabster cannot re-hide them.
func unhideOneDriveAppRoot(cr *chrome.Chrome) uiauto.Action {
	const unhideExpr = `(() => {
		const unhide = () => {
			for (const el of document.querySelectorAll('#appRoot[aria-hidden="true"], [class*="body_"][aria-hidden="true"]')) {
				el.removeAttribute('aria-hidden');
			}
		};
		unhide();
		if (!window.__tastOneDriveUnhideObserver && document.documentElement) {
			window.__tastOneDriveUnhideObserver = new MutationObserver(() => unhide());
			window.__tastOneDriveUnhideObserver.observe(document.documentElement, {
				attributes: true,
				attributeFilter: ['aria-hidden'],
				childList: true,
				subtree: true,
			});
		}
	})()`

	return func(ctx context.Context) error {
		targets, err := cr.FindTargets(ctx, chrome.MatchTargetURLPrefix("https://onedrive.live.com/"))
		if err != nil || len(targets) == 0 {
			return nil
		}
		for _, t := range targets {
			if t.Type != "page" && t.Type != "app" {
				continue
			}
			if strings.Contains(t.URL, "odspserviceworkerproxy") {
				continue
			}
			func() {
				evalCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				conn, err := cr.NewConnForTarget(evalCtx, chrome.MatchTargetID(t.TargetID))
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.Eval(evalCtx, unhideExpr, nil)
			}()
		}
		return nil
	}
}

// maybeDismissOneDriveAd closes any optional ad dialog or teaching bubble
// ("ms-TeachingBubble-content") that may pop up in OneDrive without failing if
// absent.
func maybeDismissOneDriveAd(ui *uiauto.Context, oneDriveUIAncestor *nodewith.Finder, timeout time.Duration) uiauto.Action {
	oneDriveAdDialog := nodewith.Role(role.AlertDialog).Ancestor(oneDriveUIAncestor).First()
	oneDriveCloseAdButton := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("(?i)^(Close|Got it|Maybe later)$")).Ancestor(oneDriveAdDialog).First()
	teachingBubble := nodewith.HasClass("ms-TeachingBubble-content").Ancestor(oneDriveUIAncestor).First()
	teachingBubbleCloseButton := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("(?i)^(Close|Got it|Maybe later)$")).Ancestor(teachingBubble).First()

	return func(ctx context.Context) error {
		if err := ui.WaitUntilExists(oneDriveUIAncestor)(ctx); err != nil {
			return errors.Wrap(err, "failed to find the OneDrive context")
		}

		if _, err := ui.WithTimeout(timeout).FindAnyExists(ctx, oneDriveAdDialog, teachingBubble); err == nil {
			if ui.Exists(oneDriveCloseAdButton)(ctx) == nil {
				if err := ui.LeftClickUntil(oneDriveCloseAdButton, ui.Gone(oneDriveAdDialog))(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to dismiss OneDrive alert dialog: ", err)
				}
			}
			if ui.Exists(teachingBubbleCloseButton)(ctx) == nil {
				if err := ui.LeftClickUntil(teachingBubbleCloseButton, ui.Gone(teachingBubble))(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to dismiss OneDrive teaching bubble: ", err)
				}
			}
		}
		return nil
	}
}

// findAndOpenFileInMyFiles locates the target presentation in OneDrive "My
// files" (matching either stem or stem+".pptx") and clicks it until the
// presentation window opens.
func findAndOpenFileInMyFiles(ui *uiauto.Context, cr *chrome.Chrome, oneDriveRoot *nodewith.Finder, fileName string) uiauto.Action {
	fileStem := strings.TrimSuffix(fileName, ".pptx")
	fileStemRegex := regexp.MustCompile("^" + regexp.QuoteMeta(fileStem) + `(\.pptx)?$`)
	fileLink := nodewith.Ancestor(oneDriveRoot).Role(role.Link).NameRegex(fileStemRegex).First()
	fileButton := nodewith.Ancestor(oneDriveRoot).Role(role.Button).NameRegex(fileStemRegex).First()
	fileText := nodewith.Ancestor(oneDriveRoot).Role(role.StaticText).NameRegex(fileStemRegex).First()
	fileCandidates := []*nodewith.Finder{fileLink, fileButton, fileText}

	gridContainer := nodewith.Role(role.Grid).Ancestor(oneDriveRoot).First()
	tableContainer := nodewith.Role(role.Table).Ancestor(oneDriveRoot).First()
	listContainer := nodewith.Role(role.List).Ancestor(oneDriveRoot).First()

	return func(ctx context.Context) error {
		var foundFile *nodewith.Finder
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			_ = unhideOneDriveAppRoot(cr)(ctx)
			var findErr error
			foundFile, findErr = ui.WithTimeout(time.Second).FindAnyExists(ctx, fileCandidates...)
			return findErr
		}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second}); err != nil {
			container, containerErr := ui.FindAnyExists(ctx, gridContainer, tableContainer, listContainer)
			if containerErr != nil {
				return errors.Wrap(err, "failed to find file list or target file in My files")
			}
			kb, kbErr := input.VirtualKeyboard(ctx)
			if kbErr != nil {
				return errors.Wrap(kbErr, "failed to get keyboard")
			}
			defer kb.Close(ctx)

			dataRow := nodewith.Role(role.Row).Ancestor(container).Nth(1)
			if ui.Exists(dataRow)(ctx) != nil {
				dataRow = nodewith.Role(role.Row).Ancestor(container).First()
			}
			if rowErr := ui.DoDefault(dataRow)(ctx); rowErr != nil {
				return errors.Wrap(rowErr, "failed to focus row in My files")
			}
			if scrollErr := ui.RetryUntil(kb.AccelAction("down"), func(ctx context.Context) error {
				var findErr error
				foundFile, findErr = ui.WithTimeout(500*time.Millisecond).FindAnyExists(ctx, fileCandidates...)
				return findErr
			})(ctx); scrollErr != nil {
				return errors.Wrap(scrollErr, "failed to find target file in My files")
			}
		}

		_ = ui.ScrollToVisible(foundFile)(ctx)
		return ui.WithInterval(3*time.Second).LeftClickUntil(
			foundFile,
			ui.Exists(ms365.Microsoft365WindowFinder(fileName)),
		)(ctx)
	}
}

// OdfsStayInM365Pwa verifies that the user stays within the M365 PWA when they
// create new files or open existing files from within the PWA using different
// Microsoft account types (consumer account, M365 business standard license).
func OdfsStayInM365Pwa(ctx context.Context, s *testing.State) {
	var creds credconfig.Creds

	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	fdms := data.FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Force-install the M365 PWA and set all Clippy policies appropriately.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{
		&policy.WebAppInstallForceList{
			Val: []*policy.WebAppInstallForceListValue{
				{
					Url:                    "https://www.microsoft365.com/?from=Homescreen",
					DefaultLaunchContainer: "window",
					CreateDesktopShortcut:  true,
					CustomName:             "Microsoft 365",
					FallbackAppName:        "Microsoft 365",
					CustomIcon: &policy.WebAppInstallForceListValueCustomIcon{
						Hash: "",
						Url:  "",
					},
				},
			},
		},
		&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
		&policy.MicrosoftOneDriveMount{Val: "automated"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"},
	}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}
	// Uninstall the force-installed PWA but keeping the ODFS extension installed by serving Clippy policies without the WebAppInstallForceList policy.
	defer policyutil.ServeAndRefresh(cleanupCtx, fdms, cr, []policy.Policy{&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
		&policy.MicrosoftOneDriveMount{Val: "automated"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"}})

	param := s.Param().(userParamStayInM365)
	if param.isConsumerMicrosoft {
		accountPool := s.RequiredVar("onedrive.accountPool")
		msCreds, err := credconfig.PickRandomCreds(accountPool)
		if err != nil {
			s.Fatal("Failed to get the user/passwd for Microsoft 365: ", err)
		}
		creds = msCreds
	} else {
		creds.User = s.RequiredVar(param.username)
		creds.Pass = s.RequiredVar(param.password)
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_stay_in_m365_pwa")

	// Connect to OneDrive.
	ms365App, err := ms365.AppWithCreds(ctx, tconn, creds)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	if err := office.ConnectToOneDrive(cr, tconn, files, ms365App)(ctx); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

	if err := files.OpenOneDrive()(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	odfsToken, err := files.GetOdfsFuseboxToken(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get ODFS key: ", err)
	}

	// Delete old files from OneDrive which were created by the test but could not be deleted previously.
	defer func(ctx context.Context) {
		dir := filepath.Join(filemanager.FuseboxDirPath, odfsToken)
		dirEntries, err := onedrive.FindEntriesOlderThanHalfAnHour(dir)
		if err != nil {
			s.Log("Failed to list old files: ", err)
		}

		presentationRegex := regexp.MustCompile(`^Presentation.*\.pptx$`)
		for _, f := range dirEntries {
			if !presentationRegex.MatchString(f.Name()) {
				continue
			}

			testing.ContextLog(ctx, "Deleting old file: ", f.Name())
			if err := os.RemoveAll(filepath.Join(dir, f.Name())); err != nil {
				testing.ContextLogf(ctx, "Failed to remove file: %q - %v", f.Name(), err)
			}
		}
	}(cleanupCtx)

	// Create a new file from within the M365 PWA.
	if err := apps.Launch(ctx, tconn, apps.Microsoft365.ID); err != nil {
		s.Fatal("Failed to launch Microsoft 365: ", err)
	}

	m365Window := nodewith.Role(role.Window).HasClass("BrowserRootView").NameRegex(regexp.MustCompile(`(?i)(Microsoft Copilot|Microsoft 365|microsoft365\.com)`)).First()
	m365PWAToolbar := nodewith.Role(role.Pane).HasClass("WebAppFrameToolbarView").Ancestor(m365Window)
	m365Context := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile(`(?i).*(Microsoft Copilot|Microsoft 365).*`)).First()

	powerPointWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameContaining("PowerPoint").First()
	powerPointPWAToolbar := nodewith.Role(role.Pane).HasClass("WebAppFrameToolbarView").Ancestor(powerPointWindow)
	powerPointContext := nodewith.Role(role.RootWebArea).NameContaining("PowerPoint").First()
	newPresentationLink := nodewith.Ancestor(powerPointContext).Role(role.Button).NameRegex(regexp.MustCompile(`(?i).*blank presentation.*`)).First()

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Create a new PowerPoint presentation in M365",
		ui.WaitUntilExists(m365PWAToolbar),
		ui.WaitUntilExists(m365Context),
		maybeDismissCopilotPrompts(ui, m365Context),
		openAppFromCopilotLauncher(ui, m365Context, "PowerPoint", powerPointWindow),
		ui.WaitUntilExists(powerPointPWAToolbar),
		ui.WaitUntilExists(powerPointContext),
		ui.WaitUntilExists(newPresentationLink),
		// Clicking "Create blank presentation" changes its label to "Creating
		// presentation", so wait for the button to disappear to avoid creating
		// duplicate files.
		ui.WithInterval(2*time.Second).LeftClickUntil(newPresentationLink, ui.Gone(newPresentationLink)),
	)(ctx); err != nil {
		s.Fatal("Failed to create a new PowerPoint presentation from M365: ", err)
	}

	// Verify that the document was opened within the M365 PWA and close the window.
	presentationWindowRegex := regexp.MustCompile(`^Presentation.*?\.pptx`)
	presentationFinder := ms365.Microsoft365WindowFinderWithRegex(presentationWindowRegex).First()
	if err := ui.WithTimeout(time.Minute).WaitUntilExists(presentationFinder)(ctx); err != nil {
		s.Fatal("Failed to wait for the new presentation window: ", err)
	}
	fileInfo, err := ui.Info(ctx, presentationFinder)
	if err != nil || fileInfo == nil {
		s.Fatal("Failed to find the new presentation: ", err)
	}

	fileName := presentationWindowRegex.FindString(fileInfo.Name)
	if err := ms365App.WaitForMicrosoft365EditorWindowAndClose(tconn, fileName)(ctx); err != nil {
		s.Fatal("Failed to close M365 window: ", err)
	}

	// Try to remove the just created file from OneDrive during cleanup.
	defer func(ctx context.Context) {
		fullFilePath := filepath.Join(filemanager.FuseboxDirPath, odfsToken, fileName)
		if err := os.RemoveAll(fullFilePath); err != nil {
			s.Log("Failed to remove the fresh file from OneDrive: ", err)
		}
	}(cleanupCtx)

	if err := apps.Close(ctx, tconn, apps.Microsoft365.ID); err != nil {
		s.Fatal("Failed to close Microsoft 365: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get windows"))
		}
		for _, w := range ws {
			if w.AppID == apps.Microsoft365.ID {
				return errors.Errorf("Microsoft 365 window %q (id=%d) is still open", w.Title, w.ID)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed waiting for Microsoft 365 windows to close: ", err)
	}

	// Re-open the file from M365.
	if err := apps.Launch(ctx, tconn, apps.Microsoft365.ID); err != nil {
		s.Fatal("Failed to launch Microsoft 365: ", err)
	}

	oneDriveWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameRegex(oneDriveWindowTitleRegex).First()
	oneDrivePWAToolbar := nodewith.Role(role.Pane).HasClass("WebAppFrameToolbarView").Ancestor(oneDriveWindow)
	oneDriveRoot := nodewith.Role(role.RootWebArea).NameRegex(oneDriveWindowTitleRegex).First()
	myFilesLink := nodewith.Ancestor(oneDriveRoot).Role(role.Link).Name("My files").First()
	myFilesPageRoot := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)^My files - OneDrive")).First()
	myFilesGrid := nodewith.Ancestor(oneDriveRoot).Role(role.Grid).Name("My files").First()
	myFilesHeading := nodewith.Ancestor(oneDriveRoot).Role(role.Heading).Name("My files").First()

	if err := uiauto.Combine("Open my files in OneDrive",
		ui.WaitUntilExists(m365PWAToolbar),
		ui.WaitUntilExists(m365Context),
		maybeDismissCopilotPrompts(ui, m365Context),
		// Open OneDrive twice when it opens in a separate M365 PWA window, as the
		// newly created presentation may not appear on the very first load.
		openAppFromCopilotLauncher(ui, m365Context, "OneDrive", oneDriveWindow),
		ui.WaitUntilExists(oneDrivePWAToolbar),
		ui.WaitUntilExists(oneDriveRoot),
		func(ctx context.Context) error {
			if ui.Exists(m365Context)(ctx) != nil {
				return nil
			}
			return uiauto.Combine("Re-open OneDrive PWA window",
				closeMicrosoftOneDriveWindow(tconn),
				openAppFromCopilotLauncher(ui, m365Context, "OneDrive", oneDriveWindow),
				ui.WaitUntilExists(oneDrivePWAToolbar),
				ui.WaitUntilExists(oneDriveRoot),
			)(ctx)
		},
		unhideOneDriveAppRoot(cr),
		maybeDismissOneDriveAd(ui, oneDriveRoot, 5*time.Second),
		ui.RetryUntil(unhideOneDriveAppRoot(cr), ui.Exists(myFilesLink)),
		ui.WithInterval(2*time.Second).LeftClickUntil(myFilesLink, func(ctx context.Context) error {
			_ = unhideOneDriveAppRoot(cr)(ctx)
			_, err := ui.WithTimeout(time.Second).FindAnyExists(ctx, myFilesPageRoot, myFilesGrid, myFilesHeading)
			return err
		}),
		unhideOneDriveAppRoot(cr),
		maybeDismissOneDriveAd(ui, oneDriveRoot, 2*time.Second),
		unhideOneDriveAppRoot(cr),
		findAndOpenFileInMyFiles(ui, cr, oneDriveRoot, fileName),
	)(ctx); err != nil {
		s.Fatal("Failed to open the just created file in OneDrive: ", err)
	}

	if err := ms365App.WaitForMicrosoft365EditorWindowAndClose(tconn, fileName)(ctx); err != nil {
		s.Fatal("Failed to close M365 window: ", err)
	}
}
