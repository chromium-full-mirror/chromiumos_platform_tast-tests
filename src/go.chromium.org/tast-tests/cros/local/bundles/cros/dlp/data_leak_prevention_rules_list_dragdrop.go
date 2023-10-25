// Copyright 2021 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const content = "example.com"

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListDragdrop,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with clipboard blocked restriction by drag and drop",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"ayaelattar@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:hw_agnostic"},
		Data:         []string{"draggable_link.html", "text_2.html", "editable_text_box.html"},
		Params: []testing.Param{{
			Name:      "ash_blocked",
			ExtraAttr: []string{"group:mainline"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val:       browser.TypeAsh,
		}, {
			Name:              "lacros_blocked",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}}})
}

func DataLeakPreventionRulesListDragdrop(ctx context.Context, s *testing.State) {
	// Reserve time for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	allowedServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer allowedServer.Close()

	blockedServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer blockedServer.Close()
	srcURL := blockedServer.URL + "/draggable_link.html"

	dstServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer dstServer.Close()

	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, policy.PopulateClipboardBlockPolicy(blockedServer.URL, dstServer.URL)); err != nil {
		s.Fatal("Failed to serve and verify the DLP policy: ", err)
	}

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	revertZoom, err := display.MinimizePrimaryDisplayZoomFactor(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to set the zoom factor of the primary display to minimum: ", err)
	}
	defer revertZoom(cleanupCtx, tconn)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)
	if err := cr.ResetState(ctx); err != nil {
		s.Fatal("Failed to reset the Chrome: ", err)
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open the destination browser: ", err)
	}
	defer func(ctx context.Context) {
		if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
			// The Lacros browser is not closed in other places in the test.
			s.Error("The Lacros browser probably crashed: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	dstURL := dstServer.URL + "/editable_text_box.html"
	dstConn, err := br.NewConn(ctx, dstURL)
	if err != nil {
		s.Fatalf("Failed to open page %q: %v", dstURL, err)
	}
	defer dstConn.Close()

	if err := webutil.WaitForQuiescence(ctx, dstConn, 10*time.Second); err != nil {
		s.Fatalf("Failed to wait for %q to achieve quiescence: %v", dstURL, err)
	}

	srcConn, err := br.NewConn(ctx, srcURL, browser.WithNewWindow())
	if err != nil {
		s.Fatalf("Failed to open page %q: %v", srcURL, err)
	}
	defer srcConn.Close()

	if err := webutil.WaitForQuiescence(ctx, srcConn, 10*time.Second); err != nil {
		s.Fatalf("Failed to wait for %q to achieve quiescence: %v", srcURL, err)
	}

	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enter into the overview mode: ", err)
	}

	// Snap the source window to the right.
	w1, err := ash.FindFirstWindowInOverview(ctx, tconn)
	if err != nil {
		s.Fatalf("Failed to find the %s window in the overview mode: %v", srcURL, err)
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, w1.ID, ash.WindowStateSecondarySnapped); err != nil {
		s.Fatalf("Failed to snap the %s window to the right: %v", srcURL, err)
	}

	// Snap the destination window to the left.
	w2, err := ash.FindFirstWindowInOverview(ctx, tconn)
	if err != nil {
		s.Fatalf("Failed to find the %s window in the overview mode: %v", dstURL, err)
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, w2.ID, ash.WindowStatePrimarySnapped); err != nil {
		s.Fatalf("Failed to snap the %s window to the left: %v", dstURL, err)
	}

	// Activate the drag destination window so coordinates get updates.
	if err := w2.ActivateWindow(ctx, tconn); err != nil {
		s.Fatalf("Failed to activate the %s window: %v", srcURL, err)
	}

	if err := dragdrop.WaitForStableCoordinates(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for the coordinates for the drop textfield gets stable: ", err)
	}

	// Activate the drag source window.
	if err := w1.ActivateWindow(ctx, tconn); err != nil {
		s.Fatalf("Failed to activate the %s window: %v", srcURL, err)
	}

	s.Log("Draging and dropping content")
	// Root node of the frame with the title "Editable Text Box" in ash or lacros.
	browserRoot := nodewith.ClassNameRegex(regexp.MustCompile("(BrowserFrame)|(ExoShellSurface-.*)")).NameRegex(regexp.MustCompile(".*Editable Text Box.*"))
	dstNode := nodewith.Name("textarea").Role(role.TextField).State(state.Editable, true).Ancestor(browserRoot)
	if err := dragdrop.DragDrop(ctx, tconn, content, dstNode); err != nil {
		s.Error("Failed to drag drop content: ", err)
	}

	s.Log("Checking notification")
	ui := uiauto.New(tconn)

	// Verify notification bubble.
	parsedSrcURL, err := url.Parse(blockedServer.URL)
	if err != nil {
		s.Fatalf("Failed to parse blocked server URL %s: %v", blockedServer.URL, err)
	}

	notifError := clipboard.CheckClipboardBubble(ctx, ui, parsedSrcURL.Hostname())

	if notifError != nil {
		s.Error("Expected notification but found an error: ", notifError)
	}

	// Check dropped content.
	contentNode := nodewith.NameContaining(content).Role(role.InlineTextBox).State(state.Editable, true).First()
	dropError := ui.WaitUntilExists(contentNode)(ctx)

	if dropError == nil {
		s.Error("Content was pasted but should have been blocked")
	}
}
