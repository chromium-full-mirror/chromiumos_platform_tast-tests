// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscujv2

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// MapPDFFile specifies the file name of the map PDF file.
	MapPDFFile = "map.pdf"
	// AnimationFile specifies the file name of animation html.
	AnimationFile = "animation.html"

	// The following URLs are opened on the desks.
	crosVideoURL               = "https://crosvideo.appspot.com/?codec=h264_60&loop=true&mute=true"
	chromiumIssueURL           = "https://bugs.chromium.org/p/chromium/issues?q=status:open"
	chromeWebStoreExtensionURL = "https://chromewebstore.google.com/category/extensions"
	youtubeURL                 = "https://www.youtube.com"
	webGLURL                   = "https://webglsamples.org/aquarium/aquarium.html?numFish=1000"
	largeGoogleDocsURL         = "https://docs.google.com/document/d/19R_RWgGAqcHtgXic_YPQho7EwZyUAuUZyBq4n_V-BJ0/edit"
	// pictureGoogleDocsCopyURL is the URL that triggers the "Make a copy" prompt for the picture Google Docs file,
	// making it easier for copying the file.
	pictureGoogleDocsCopyURL = "https://docs.google.com/document/d/1htcEghziiE5Ts1mPRzyTmB3ad11H0hA6FzsO-DgvQ-w/copy"

	// The window title of |crosVideoURL|.
	crosVideoTitle = "CrosVideo Test"
	// The window title of |largeGoogleDocsURL|.
	largeGoogleDocsTitle = "War and Peace, by Leo Tolstoy"
	// The window title of |pictureGoogleDocsURL|.
	pictureGoogleDocsTitle = "test: 5 images, 2 graphs"
)

// openDesk creates and initializes a desk based on the info in |desk|.
// |i| is the index of the new desk. If i is 0, the desk will just be
// initialized, because the first desk is created and activated by default.
// If |individualWindows| is true, each url in |urls| will be placed in a separate window.
// If |individualWindows| is false, new tabs will be opened in the first browser window.
// Each successive call to openDesk must have an |i| value exactly 1 more than in the previous
// call, with the first call to this function expected to be 0.
func openDesk(ctx context.Context, tconn *chrome.TestConn, cs ash.ConnSource, urls []string, individualWindows bool, expectedNumWindows, i int) ([]cuj.TabConn, error) {
	if i != 0 {
		prepareDeskCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		if err := ash.CreateNewDesk(prepareDeskCtx, tconn); err != nil {
			return nil, errors.Wrapf(err, "failed to create desk %d", i)
		}
		if err := ash.ActivateDeskAtIndex(prepareDeskCtx, tconn, i); err != nil {
			return nil, errors.Wrapf(err, "failed to activate desk %d", i)
		}
	}

	deskTabs, err := cuj.NewTabsByURLs(ctx, cs, individualWindows, urls)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open urls for desk %d", i)
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return nil, err
	}
	if numWindows := len(ws); numWindows != expectedNumWindows {
		return nil, errors.Errorf("unexpected number of open windows after setting up desk %d: got %d windows, expected %d windows", i, numWindows, expectedNumWindows)
	}

	return deskTabs, nil
}

// setUpDesks creates 3 desks in addition to the initial default desk,
// and opens up a variety of windows on each desk. At the end of
// setUpDesks, there will be a total of 4 desks, with rightmost desk
// being active. This function returns a list of actions to be performed
// on the corresponding desk, as well as the total number of windows that
// should be open after setUpDesks completes.
// This function also returns the cleanup function to delete the Google Docs
// file created by setUpDesks and close the Gallery app.
//
// Desks are arranged based on the following:
// Desk 1:
//   - Windows: 1
//   - User Input: Mouse Scroll Wheel
//
// Desk 2:
//   - Windows: 1
//   - User Input: None
//
// Desk 3:
//   - Windows: 2
//   - User Input: Trackpad Scroll
//
// Desk 4:
//   - Windows: 1
//   - User Input: Keyboard typing, Mouse Movement
func setUpDesks(ctx context.Context, cr *chrome.Chrome, kw *input.KeyboardEventWriter, pc pointer.Context, mw *input.MouseEventWriter, tpw *input.TrackpadEventWriter, tw *input.TouchEventWriter, testParam TestParam) (_ []action.Action, _ int, _ func(ctx context.Context) error, retErr error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, 0, nil, errors.Wrap(err, "failed to create Test API connection")
	}

	// Create a separate desks-setup deadline. 15 minutes should be
	// enough time to open all of the windows and desks. This limits
	// the time that desk setup can take, to ensure we have time
	// left over for the test itself.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return nil, 0, nil, errors.Wrap(err, "failed to get the primary display info")
	}

	ui := uiauto.New(tconn)

	desk1VisitAction := func(ctx context.Context) error {
		if err := switchToWindow(ctx, cr, crosVideoTitle); err != nil {
			return errors.Wrap(err, "failed to switch to CrosVideo")
		}
		// GoBigSleepLint: sleep for 5 seconds to let the video play.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		// This includes the 3 websites defined in urls, the additional CrosVideo tab that opened in recorder, and any custom extra URLs.
		totalTabs := 4 + len(testParam.ExtraURLsForDesk1)
		scrollDownAndUp := func(ctx context.Context) error {
			for tabIdx := 0; tabIdx < totalTabs; tabIdx++ {
				if err := switchToTab(ctx, tconn, info, tabIdx); err != nil {
					return errors.Wrap(err, "failed to switch tab")
				}

				if err := inputsimulations.ScrollMouseDownFor(ctx, mw, 500*time.Millisecond, 5*time.Second); err != nil {
					return errors.Wrap(err, "failed to scroll down with mouse")
				}
				if err := inputsimulations.ScrollMouseUpFor(ctx, mw, 500*time.Millisecond, 5*time.Second); err != nil {
					return errors.Wrap(err, "failed to scroll up with mouse")
				}
			}
			return nil
		}
		// Repeat scrolling mouse actions 3 times for each tab.
		return uiauto.Repeat(3, scrollDownAndUp)(ctx)
	}
	desk2VisitAction := uiauto.Sleep(10 * time.Second)
	desk3VisitAction := func(ctx context.Context) error {
		docsWindow, err := ash.FindWindow(ctx, tconn, func(window *ash.Window) bool {
			return strings.Contains(window.Title, largeGoogleDocsTitle)
		})
		if err != nil {
			return errors.Wrap(err, "failed to find the Google Docs window")
		}
		docsBounds := docsWindow.TargetBounds
		if err := mouse.Move(tconn, docsBounds.CenterPoint(), 500*time.Millisecond)(ctx); err != nil {
			return errors.Wrap(err, "failed to move mouse to center of Google Docs window")
		}
		if err := inputsimulations.ScrollDownFor(ctx, tpw, tw, time.Second, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to scroll down with trackpad")
		}

		return toggleLauncher(ctx, tconn)
	}

	const notes = "The quick brown fox jumps over the lazy dog in the afternoon on Saturday!"
	desk4VisitAction := uiauto.Combine("edit doc and click file menu",
		googledocs.EditDoc(tconn, kw, notes),
		googledocs.ClickFileMenuButtonWithFinder(ui, pc),
	)

	var (
		tabs             []cuj.TabConn
		totalOpenWindows int
		onVisitActions   []action.Action
		cleanups         []action.Action
	)
	for i, desk := range []struct {
		urls               []string      // A list of urls to open for this desk.
		onVisitAction      action.Action // Unique user input action to perform on this desk.
		individualWindows  bool          // whether the |urls| should be placed in separate windows.
		expectedNumWindows int           // Expected number of windows that should be open after desk setup.
	}{
		{
			urls: append([]string{
				chromiumIssueURL,
				chromeWebStoreExtensionURL,
				youtubeURL,
			}, testParam.ExtraURLsForDesk1...),
			onVisitAction:      desk1VisitAction,
			expectedNumWindows: 1,
		},
		{
			urls:               []string{webGLURL},
			onVisitAction:      desk2VisitAction,
			individualWindows:  true,
			expectedNumWindows: 1,
		},
		{
			urls:               []string{testParam.AnimationURL, largeGoogleDocsURL},
			onVisitAction:      desk3VisitAction,
			individualWindows:  true,
			expectedNumWindows: 2,
		},
		{
			urls:               []string{pictureGoogleDocsCopyURL},
			onVisitAction:      desk4VisitAction,
			individualWindows:  true,
			expectedNumWindows: 1,
		},
	} {
		totalOpenWindows += desk.expectedNumWindows
		deskTabs, err := openDesk(ctx, tconn, cr, desk.urls, desk.individualWindows, totalOpenWindows, i)
		if err != nil {
			return nil, totalOpenWindows, nil, errors.Wrapf(err, "failed to complete setup for desk %d", i)
		}

		if desk.urls[0] == pictureGoogleDocsCopyURL {
			cleanupCtx := ctx
			ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			cleanupPDF, err := openPDFFile(ctx, cr, tconn, MapPDFFile)
			if err != nil {
				return nil, totalOpenWindows, nil, errors.Wrap(err, "failed to open PDF file")
			}
			defer func(ctx context.Context) {
				if retErr != nil {
					cleanupPDF(ctx)
				}
			}(cleanupCtx)
			cleanups = append(cleanups, cleanupPDF)
			totalOpenWindows++

			cleanupDocs, err := copyDocsFile(ctx, cr, kw, pictureGoogleDocsTitle)
			if err != nil {
				return nil, totalOpenWindows, nil, errors.Wrap(err, "failed to copy docs file")
			}
			defer func(ctx context.Context) {
				if retErr != nil {
					cleanupDocs(ctx)
				}
			}(cleanupCtx)
			cleanups = append(cleanups, cleanupDocs)
		}
		onVisitActions = append(onVisitActions, desk.onVisitAction)
		tabs = append(tabs, deskTabs...)
	}

	if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
		// Split the windows in desk 3.
		if strings.Contains(w.Title, AnimationFile) {
			return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStatePrimarySnapped)
		}
		if strings.Contains(w.Title, largeGoogleDocsTitle) {
			return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateSecondarySnapped)
		}
		return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
	}); err != nil {
		return onVisitActions, totalOpenWindows, nil, errors.Wrap(err, "failed to set each window state")
	}

	// Close connections to each tab because we don't need them.
	for _, tab := range tabs {
		if err := tab.Conn.Close(); err != nil {
			return nil, totalOpenWindows, nil, errors.Wrapf(err, "failed to close connection to %s", tab.URL)
		}
	}

	return onVisitActions, totalOpenWindows, uiauto.Combine("cleanup files", cleanups...), nil
}

// copyDocsFile switches to the Google docs window by window title |docWindowTitle| and copies the docs file.
// This function also returns the cleanup function to delete the copied doc file.
func copyDocsFile(ctx context.Context, cr *chrome.Chrome, kw *input.KeyboardEventWriter, docWindowTitle string) (_ func(context.Context) error, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := switchToWindow(ctx, cr, docWindowTitle); err != nil {
		return nil, errors.Wrap(err, "failed to switch to docs window")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating test API connection failed")
	}

	ui := uiauto.New(tconn)
	copyButton := nodewith.Name("Make a copy").Role(role.Button)
	if err := ui.DoDefault(copyButton)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to open the copied docs")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := googledocs.DeleteDoc(tconn)(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to delete docs: ", err)
			}
		}
	}(cleanupCtx)

	return func(ctx context.Context) error {
		copyFileTitle := "Copy of " + docWindowTitle
		if err := switchToWindow(ctx, cr, copyFileTitle); err != nil {
			return errors.Wrap(err, "failed to switch to docs window")
		}
		if err := googledocs.DeleteDoc(tconn)(ctx); err != nil {
			return errors.Wrap(err, "failed to delete docs")
		}
		return nil
	}, nil

}

// openPDFFile opens the specific pdf file from Files app identified by its name |pdfFileName|.
// This function also returns the cleanup function to close Gallery app.
func openPDFFile(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, pdfFileName string) (_ func(context.Context) error, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch the Files app")
	}
	defer filesApp.Close(cleanupCtx)

	if err := uiauto.NamedCombine("open PDF file",
		filesApp.OpenDownloads(),
		filesApp.OpenFile(pdfFileName),
	)(ctx); err != nil {
		return nil, err
	}

	gallery, err := galleryapp.ConnectToApp(ctx, cr, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to gallery app")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := gallery.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close gallery app: ", err)
			}
		}
	}(cleanupCtx)

	if err := uiauto.NamedCombine("maximize gallery window",
		gallery.DismissPDFDialog(),
		gallery.MaximizeWindow(),
		gallery.WaitPDFOpened(),
		gallery.WaitForGalleryQuiescence(cr),
	)(ctx); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		if err := gallery.Close(ctx); err != nil {
			return errors.Wrap(err, "failed to close gallery app")
		}
		return nil
	}, nil
}

// switchToWindow switches to the specific window identified by its title |windowTitle|.
func switchToWindow(ctx context.Context, cr *chrome.Chrome, windowTitle string) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	uiHandler, err := cuj.NewClamshellActionHandler(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "failed to create clamshell action handler")
	}
	defer uiHandler.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "creating test API connection failed")
	}

	chromeApp, err := apps.ChromeOrChromium(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find the Chrome app")
	}
	if err := uiHandler.SwitchToAppWindowByName(chromeApp.Name, windowTitle)(ctx); err != nil {
		return errors.Wrapf(err, "failed to switch to %q", windowTitle)
	}
	return nil
}

// switchToTab switches to the |tabIdx|th tab in the currently active window and moves mouse to the center
// of the display.
func switchToTab(ctx context.Context, tconn *chrome.TestConn, info *display.Info, tabIdx int) error {
	ui := uiauto.New(tconn)
	tabToClick := nodewith.HasClass("Tab").Nth(tabIdx)
	return action.Combine(
		"click on tab and move mouse back to the center of the display",
		ui.MouseMoveTo(tabToClick, 20*time.Millisecond),
		ui.LeftClick(tabToClick),
		mouse.Move(tconn, info.Bounds.CenterPoint(), 20*time.Millisecond),
	)(ctx)
}

// toggleLauncher toggles the launcher for ash input metrics.
func toggleLauncher(ctx context.Context, tconn *chrome.TestConn) error {
	return uiauto.NamedCombine("toggle launcher",
		launcher.Open(tconn),
		launcher.CloseBubbleLauncher(tconn),
	)(ctx)
}
