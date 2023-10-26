// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		Func:         DataLeakPreventionRulesListClipboardMixedTypeBrowsers,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with clipboard blocked restrictions from Ash to Lacros and vice versa",
		Contacts: []string{
			"chromeos-dlp@google.com", // Feature owners.
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome", "lacros"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Data:         []string{"text_1.html", "editable_text_box.html"},
		Fixture:      fixture.LacrosPolicyLoggedIn,
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			Name:      "blocked_ash_to_lacros",
			ExtraAttr: []string{"group:mainline", "informational"},
			Val: dragdrop.MixedBrowsersParams{
				Source:      dragdrop.Settings,
				Destination: dragdrop.Chrome,
			},
		}, {
			Name:      "blocked_lacros_to_ash",
			ExtraAttr: []string{"group:golden_tier"},
			Val: dragdrop.MixedBrowsersParams{
				Source:      dragdrop.Chrome,
				Destination: dragdrop.Settings,
			},
		}},
	})
}

func DataLeakPreventionRulesListClipboardMixedTypeBrowsers(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	params := s.Param().(dragdrop.MixedBrowsersParams)

	srcServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srcServer.Close()

	dstServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer dstServer.Close()

	dstURL := dstServer.URL + "/editable_text_box.html"

	srcURL := srcServer.URL + "/text_1.html"

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	dstMatch := dstServer.URL
	if params.Destination != dragdrop.Chrome {
		dstMatch = params.Destination.String()
	}

	srcMatch := srcServer.URL
	if params.Source != dragdrop.Chrome {
		srcMatch = params.Source.String()
	}

	// Reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Perform cleanup.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	if err := policyutil.ServeAndVerify(ctx, fdms, cr, policy.PopulateClipboardBlockPolicy(srcMatch, dstMatch)); err != nil {
		s.Fatal("Failed to serve and verify the DLP policy: ", err)
	}

	if params.Source == dragdrop.Chrome {
		parsedSrcURL, err := url.Parse(srcServer.URL)
		if err != nil {
			s.Fatalf("Could not parse the source url %s: %v", srcServer.URL, err)
		}
		srcMatch = parsedSrcURL.Hostname()
	}

	s.Log("Waiting for chrome.clipboard API to become available")
	if err := tconn.WaitForExpr(ctx, "chrome.clipboard"); err != nil {
		s.Fatal("Failed to wait for chrome.clipboard API to become available: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	var dstWin *ash.Window
	var srcWin *ash.Window

	// Start Ash app.
	_, err = ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open settings app: ", err)
	}

	activeWin, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active Window for OS Settings: ", err)
	}
	if params.Source == dragdrop.Settings {
		srcWin = activeWin
	} else {
		dstWin = activeWin
	}

	// Start browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeLacros)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer func(ctx context.Context) {
		if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
			// The Lacros browser is not closed in other places in the test.
			s.Error("The Lacros browser probably crashed: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_after_browser_open_error")

	if params.Source == dragdrop.Chrome {
		srcWin, err = ash.GetActiveWindow(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to set source window to lacros: ", err)
		}
	}
	if params.Destination == dragdrop.Chrome {
		dstWin, err = ash.GetActiveWindow(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to set destination window to lacros: ", err)
		}
	}
	ui := uiauto.New(tconn)

	// Setup source app.
	if err := srcWin.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Cannot activate source window: ", err)
	}
	if params.Source == dragdrop.Chrome {
		conn, err := br.NewConn(ctx, srcURL)
		if err != nil {
			s.Fatalf("Failed to open page %q: %v", srcURL, err)
		}
		defer conn.Close()

		if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
			s.Fatalf("Failed to wait for %q to achieve quiescence: %v", srcURL, err)
		}

		if err := uiauto.Combine("copy all text from source website",
			keyboard.AccelAction("Ctrl+A"),
			keyboard.AccelAction("Ctrl+C"))(ctx); err != nil {
			s.Fatal("Failed to copy text from source browser: ", err)
		}

	} else {
		searchBar := nodewith.NameContaining("Search settings").Role(role.SearchBox)
		if err := uiauto.Combine("Type text into search field and cut it",
			ui.WaitUntilExists(searchBar.Visible()),
			ui.LeftClick(searchBar),
			ui.WaitUntilExists(searchBar.Focused()),
			keyboard.TypeAction("Text to copy"),
			keyboard.AccelAction("Ctrl+A"),
			keyboard.AccelAction("Ctrl+X"))(ctx); err != nil {
			s.Fatal("Failed to type and copy text: ", err)
		}
	}

	copiedString, err := clipboard.GetClipboardContent(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get clipboard content: ", err)
	}

	// Setup destination app.
	if err := dstWin.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Cannot activate destination window: ", err)
	}

	var pageRootFinder *nodewith.Finder
	if params.Destination == dragdrop.Chrome {
		dstConn, err := br.NewConn(ctx, dstURL)
		if err != nil {
			s.Fatalf("Failed to open page %q: %v", dstURL, err)
		}
		defer dstConn.Close()

		if err := webutil.WaitForQuiescence(ctx, dstConn, 10*time.Second); err != nil {
			s.Fatalf("Failed to wait for %q to achieve quiescence: %v", dstURL, err)
		}

		// Retrieve the web page with the target text box.
		pageRootFinder = nodewith.Name("Editable Text Box").Role(role.RootWebArea)
		if err := ui.WaitUntilExists(pageRootFinder)(ctx); err != nil {
			s.Fatal("Unable to find target page: ", err)
		}

		textBoxNode := nodewith.Name("textarea").Role(role.TextField).State(state.Editable, true).First()
		if err := uiauto.Combine("pasting into text box",
			ui.WaitUntilExists(textBoxNode.Visible()),
			ui.LeftClick(textBoxNode),
			ui.WaitUntilExists(textBoxNode.Focused()),
			keyboard.AccelAction("Ctrl+V"),
		)(ctx); err != nil {
			s.Fatal("Failed to paste into text box: ", err)
		}
	} else {

		// Retrieve the OS Settings page with the target text box.
		pageRootFinder = nodewith.Name("Settings").Role(role.RootWebArea)
		if err := ui.WaitUntilExists(pageRootFinder)(ctx); err != nil {
			s.Fatal("Unable to find target page: ", err)
		}

		if err := uiauto.Combine("Paste text into search",
			keyboard.AccelAction("Ctrl+F"),
			keyboard.AccelAction("Ctrl+V"))(ctx); err != nil {
			s.Fatal("Failed paste text: ", err)
		}
	}

	notifError := clipboard.CheckClipboardBubble(ctx, ui, srcMatch)
	if notifError != nil {
		s.Error("Expected notification but found an error: ", notifError)
	}

	// Check pasted content.
	pastedError := clipboard.CheckPastedContent(ctx, ui, copiedString, pageRootFinder)
	if pastedError == nil {
		s.Error("Content was pasted but should have been blocked")
	}
}
