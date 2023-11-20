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

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/clipboard"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/dragdrop"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
		Data:    []string{"draggable_link.html", "text_2.html", "editable_text_box.html"},
		Fixture: fixture.LacrosPolicyLoggedIn,
		Timeout: 3 * time.Minute,
		Params: []testing.Param{{
			Name:      "blocked_ash_to_lacros",
			ExtraAttr: []string{"group:golden_tier"},
			Val: dragdrop.MixedBrowsersParams{
				Source: dragdrop.Settings,
			},
		}, {
			Name:      "blocked_lacros_to_ash",
			ExtraAttr: []string{"group:golden_tier"},
			Val: dragdrop.MixedBrowsersParams{
				Source: dragdrop.Chrome,
			},
		}},
	})
}

const sourceContent = "example.com"

func DataLeakPreventionRulesListDragdropMixedTypeBrowsers(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	params := s.Param().(dragdrop.MixedBrowsersParams)

	srcServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srcServer.Close()

	srcURL := srcServer.URL + "/draggable_link.html"

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

	// The strings to match in rules is either the app name or the page url.
	dstMatch := dstServer.URL
	if params.Source == dragdrop.Chrome {
		dstMatch = dragdrop.Settings.String()
	}

	srcMatch := srcServer.URL
	if params.Source == dragdrop.Settings {
		srcMatch = dragdrop.Settings.String()
	}

	// Perform cleanup.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	if err := policyutil.ServeAndVerify(ctx, fdms, cr, policy.PopulateClipboardBlockPolicy(srcMatch, dstMatch)); err != nil {
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
	if params.Source == dragdrop.Chrome {
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

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enter into the overview mode: ", err)
	}

	// Snap the source window to the right.
	browserWin, err := snapFirstWindowInOverview(ctx, tconn, ash.WindowStateSecondarySnapped)
	if err != nil {
		s.Fatalf("Failed to snap the %s window to the right: %s", srcURL, err)
	}

	// Snap the destination window to the left.
	_, err = snapFirstWindowInOverview(ctx, tconn, ash.WindowStatePrimarySnapped)
	if err != nil {
		s.Fatalf("Failed to snap the %s window to the left: %s", dstURL, err)
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, browserWin.ID, ash.WindowStateSecondarySnapped); err != nil {
		s.Fatal("Failed to move the browser window to the right: ", err)
	}

	// Activate the drag destination window so coordinates get updates.
	if err := browserWin.ActivateWindow(ctx, tconn); err != nil {
		s.Fatalf("Failed to activate the %s window: %v", srcURL, err)
	}

	if params.Source == dragdrop.Settings {
		if err := dragdrop.WaitForStableCoordinates(ctx, tconn); err != nil {
			s.Fatal("Failed to wait for the coordinates for the drop textfield gets stable: ", err)
		}
	}

	var dstNode *nodewith.Finder

	if params.Source == dragdrop.Chrome {
		// Activate the drag source window.
		if err := browserWin.ActivateWindow(ctx, tconn); err != nil {
			s.Fatalf("Failed to activate the %s window: %s", srcURL, err)
		}

		dstNode = ossettings.SearchLanguages
	} else {
		if err := settingsWin.ActivateWindow(ctx, tconn); err != nil {
			s.Fatal("Failed to activate the settings window: ", err)
		}

		if err := uiauto.Combine("Type text and copy it",
			keyboard.TypeAction(sourceContent),
			keyboard.AccelAction("Ctrl+A"),
		)(ctx); err != nil {
			s.Fatal("Failed to type and copy text: ", err)
		}

		browserRoot := nodewith.ClassNameRegex(regexp.MustCompile("ExoShellSurface-.*")).NameRegex(regexp.MustCompile(".*Editable Text Box.*"))
		dstNode = nodewith.Name("textarea").Role(role.TextField).State(state.Editable, true).Ancestor(browserRoot)
	}

	s.Log("Draging and dropping content")
	if err := dragdrop.DragDrop(ctx, tconn, sourceContent, dstNode); err != nil {
		s.Fatal("Failed to drag and drop content: ", err)
	}

	s.Log("Checking notification")

	srcName := srcMatch
	if params.Source == dragdrop.Chrome {
		parsedSrcURL, _ := url.Parse(srcServer.URL)
		srcName = parsedSrcURL.Hostname()
	}

	if err := clipboard.CheckClipboardBubble(ctx, ui, srcName); err != nil {
		s.Error("Couldn't check for notification: ", err)
	}

	// Check dropped content.
	contentNode := nodewith.NameContaining(sourceContent).Role(role.InlineTextBox).State(state.Editable, true).Ancestor(dstNode)

	dropError := ui.WaitUntilExists(contentNode)(ctx)

	if dropError == nil {
		s.Error("Content was pasted but should have been blocked")
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
