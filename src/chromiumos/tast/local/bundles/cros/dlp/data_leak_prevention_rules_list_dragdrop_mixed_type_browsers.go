// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"time"

	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/dlp/clipboard"
	"chromiumos/tast/local/bundles/cros/dlp/dragdrop"
	"chromiumos/tast/local/bundles/cros/dlp/policy"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/chrome/lacros"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/uiauto/state"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListDragdropMixedTypeBrowsers,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with drag and drop restrictions from Ash to Lacros and vice versa",
		Contacts: []string{
			"chromeos-dlp@google.com", // Feature owners
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome", "lacros"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic"},
		Data:    []string{"text_1.html", "text_2.html", "editable_text_box.html"},
		Fixture: "lacrosPolicyLoggedIn",
		Timeout: 3 * time.Minute,
	})
}

func DataLeakPreventionRulesListDragdropMixedTypeBrowsers(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	srcServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srcServer.Close()

	srcURL := srcServer.URL + "/text_1.html"

	dstServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer dstServer.Close()

	dstURL := dstServer.URL + "/editable_text_box.html"

	// Reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Sets the display zoom factor to minimum, to ensure that the work area
	// length is at least twice the minimum length of a browser window, so that
	// browser windows can be snapped in split view.
	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}
	zoomInitial := info.DisplayZoomFactor
	zoomMin := info.AvailableDisplayZoomFactors[0]
	if err := display.SetDisplayProperties(ctx, tconn, info.ID, display.DisplayProperties{DisplayZoomFactor: &zoomMin}); err != nil {
		s.Fatalf("Failed to set display zoom factor to minimum %f: %v", zoomMin, err)
	}
	defer display.SetDisplayProperties(cleanupCtx, tconn, info.ID, display.DisplayProperties{DisplayZoomFactor: &zoomInitial})

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	for _, param := range []struct {
		name        string
		dropAllowed bool
		src         dragdrop.AppName
		srcContent  string
	}{
		{
			name:        "blockedAshToLacros",
			dropAllowed: false,
			src:         dragdrop.Settings,
			srcContent:  "Sample text about random things.",
		},
		{
			name:        "blockedLacrosToAsh",
			dropAllowed: false,
			src:         dragdrop.Chrome,
			srcContent:  "Sample text about random things.",
		},
		{
			name:        "allowedAshToLacros",
			dropAllowed: true,
			src:         dragdrop.Settings,
			srcContent:  "Sample text about random things.",
		},
		{
			name:        "allowedLacrosToAsh",
			dropAllowed: true,
			src:         dragdrop.Chrome,
			srcContent:  "Sample text about random things.",
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// The strings to match in rules is either the app name or the page url.
			dstMatch := dstServer.URL
			if param.src == dragdrop.Chrome {
				dstMatch = dragdrop.Settings.String()
			}

			srcMatch := srcServer.URL
			if param.src == dragdrop.Settings {
				srcMatch = dragdrop.Settings.String()
			}

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			if param.dropAllowed {
				dstMatch = dstMatch + "/not_match"
			}

			if err := policyutil.ServeAndVerify(ctx, fdms, cr, policy.PopulateDLPPolicyForClipboard(srcMatch, dstMatch)); err != nil {
				s.Fatal("Failed to serve and verify the DLP policy: ", err)
			}

			s.Log("Waiting for chrome.clipboard API to become available")
			if err := tconn.WaitForExpr(ctx, "chrome.clipboard"); err != nil {
				s.Fatal("Failed to wait for chrome.clipboard API to become available: ", err)
			}

			ui := uiauto.New(tconn)

			if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osLanguages/languages", ui.Exists(nodewith.Name("Add languages").Role(role.Button))); err != nil {
				s.Fatal("Failed to launch Settings page: ", err)
			}

			if err := uiauto.Combine("open languages list",
				ui.LeftClick(ossettings.AddLanguagesButton),
				ui.WaitUntilExists(ossettings.SearchLanguages),
			)(ctx); err != nil {
				s.Fatal("Cannot open search language: ", err)
			}

			settingsWin, err := ash.GetActiveWindow(ctx, tconn)

			// Setup browser.
			var closeBr func(ctx context.Context) error
			var conn *chrome.Conn
			if param.src == dragdrop.Chrome {
				closeBr, conn, err = openWebsite(ctx, cr, browser.TypeLacros, srcURL)
				if err != nil {
					s.Fatalf("Failed to open %q: %v", srcURL, err)
				}
			} else {
				closeBr, conn, err = openWebsite(ctx, cr, browser.TypeLacros, dstURL)
				if err != nil {
					s.Fatalf("Failed to open %q: %v", dstURL, err)
				}
			}
			defer func(ctx context.Context) {
				if err := closeBr(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
					// The Lacros browser is not closed in other places in the test.
					s.Error("The Lacros browser probably crashed: ", err)
				}
			}(cleanupCtx)
			defer conn.Close()

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
				s.Fatal("Failed to enter into the overview mode: ", err)
			}

			// Snap the param.srcURL window to the right.
			browserWin, err := snapFirstWindowInOverview(ctx, tconn, ash.WindowStateRightSnapped)
			if err != nil {
				s.Fatalf("Failed to snap the %s window to the right: %s", srcURL, err)
			}

			// Snap the destination window to the left.
			_, err = snapFirstWindowInOverview(ctx, tconn, ash.WindowStateLeftSnapped)
			if err != nil {
				s.Fatalf("Failed to snap the %s window to the left: %s", dstURL, err)
			}

			if err := ash.SetWindowStateAndWait(ctx, tconn, browserWin.ID, ash.WindowStateRightSnapped); err != nil {
				s.Fatal("Failed to move the browser window to the right: ", err)
			}

			// Activate the drag destination window so coordinates get updates.
			if err := browserWin.ActivateWindow(ctx, tconn); err != nil {
				s.Fatalf("Failed to activate the %s window: %v", srcURL, err)
			}

			if param.src == dragdrop.Settings {
				if err := dragdrop.WaitForStableCoordinates(ctx, tconn); err != nil {
					s.Fatal("Failed to wait for the coordinates for the drop textfield gets stable: ", err)
				}
			}

			var dstNode *nodewith.Finder

			if param.src == dragdrop.Chrome {
				// Activate the drag source (param.srcURL) window.
				if err := browserWin.ActivateWindow(ctx, tconn); err != nil {
					s.Fatalf("Failed to activate the %s window: %s", srcURL, err)
				}

				if err = keyboard.Accel(ctx, "Ctrl+A"); err != nil {
					s.Fatal("Failed to press Ctrl+A to select all content: ", err)
				}

				dstNode = ossettings.SearchLanguages
			} else {
				if err := settingsWin.ActivateWindow(ctx, tconn); err != nil {
					s.Fatal("Failed to activate the settings window: ", err)
				}

				if err := uiauto.Combine("Type text and copy it",
					keyboard.TypeAction(param.srcContent),
					keyboard.AccelAction("Ctrl+A"),
				)(ctx); err != nil {
					s.Fatal("Failed to type and copy text: ", err)
				}

				browserRoot := nodewith.ClassNameRegex(regexp.MustCompile("ExoShellSurface-.*")).NameRegex(regexp.MustCompile(".*Editable Text Box.*"))
				dstNode = nodewith.Name("textarea").Role(role.TextField).State(state.Editable, true).Ancestor(browserRoot)
			}

			s.Log("Draging and dropping content")
			if err := dragdrop.DragDrop(ctx, tconn, param.srcContent, dstNode); err != nil {
				s.Fatal("Failed to drag and drop content: ", err)
			}

			s.Log("Checking notification")

			srcName := srcMatch
			if param.src == dragdrop.Chrome {
				parsedSrcURL, _ := url.Parse(srcServer.URL)
				srcName = parsedSrcURL.Hostname()
			}
			err = clipboard.CheckClipboardBubble(ctx, ui, srcName)

			if !param.dropAllowed && err != nil {
				s.Error("Couldn't check for notification: ", err)
			}

			if param.dropAllowed && err == nil {
				s.Error("Content pasted, expected restriction")
			}

			// Check dropped content.
			contentNode := nodewith.NameContaining(param.srcContent).Role(role.InlineTextBox).State(state.Editable, true).Ancestor(dstNode)

			dropError := ui.WaitUntilExists(contentNode)(ctx)

			if param.dropAllowed && dropError != nil {
				s.Error("Checked pasted content but found an error: ", dropError)
			}

			if !param.dropAllowed && dropError == nil {
				s.Error("Content was pasted but should have been blocked")
			}
		})
	}
}

// openWebsite opens a browser of |brType| and navigates to the |url|.
func openWebsite(ctx context.Context, cr *chrome.Chrome, brType browser.Type, url string) (uiauto.Action, *chrome.Conn, error) {
	br, closeBr, err := browserfixt.SetUp(ctx, cr, brType)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "couldn't launch the %v browser", brType)
	}

	conn, err := br.NewConn(ctx, url)
	if err != nil {
		return closeBr, nil, err
	}

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		return closeBr, conn, errors.Wrapf(err, "%q couldn't achieve quiescence", url)
	}

	return closeBr, conn, nil
}

// snapFirstWindowInOverview sets the first window in the overview to a |targetState|.
func snapFirstWindowInOverview(ctx context.Context, tconn *chrome.TestConn, targetState ash.WindowStateType) (*ash.Window, error) {
	w, err := ash.FindFirstWindowInOverview(ctx, tconn)
	if err != nil {
		return w, err
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, targetState); err != nil {
		return w, err
	}

	return w, nil
}
