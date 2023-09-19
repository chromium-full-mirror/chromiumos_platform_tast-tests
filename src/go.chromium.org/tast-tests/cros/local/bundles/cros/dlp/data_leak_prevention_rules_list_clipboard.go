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
	policyBlob "go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/clipboard"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/restrictionlevel"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
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

// A struct containing parameters for different clipboard tests.
type clipboardTestParams struct {
	name        string
	restriction restrictionlevel.RestrictionLevel
	copyAllowed bool
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListClipboard,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with clipboard warning/block restriction by copy and paste",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"ayaelattar@google.com",
			"accorsi@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:hw_agnostic"},
		Data:         []string{"text_1.html", "text_2.html", "editable_text_box.html"},
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Block users from sharing confidential information (clipboard): COM_DATPROT_CUJ3_TASK1_WF1.
			Value: "screenplay-0ebb5426-48db-4aa8-93cd-f021cc9cd0dd",
		}, {
			Key: "feature_id",
			// Block users from sharing confidential information within company (clipboard): COM_DATPROT_CUJ4_TASK1_WF1.
			Value: "screenplay-2b088681-56eb-4a9b-8ab1-73bc467ff38d",
		}, {
			Key: "feature_id",
			// Warn users from sharing confidential information (clipboard): COM_DATPROT_CUJ3_TASK2_WF1.
			Value: "screenplay-d1fd7520-180a-4344-9c68-8daf845ad5f6",
		}, {
			Key: "feature_id",
			// Warn users from sharing confidential information within company (clipboard): COM_DATPROT_CUJ4_TASK2_WF1.
			Value: "screenplay-09adb37a-9c53-4249-a417-f298ef0a2861",
		}},
		Params: []testing.Param{{
			Name:      "ash_blocked",
			ExtraAttr: []string{"group:golden_tier"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: clipboardTestParams{
				name:        "blocked",
				restriction: restrictionlevel.Blocked,
				copyAllowed: false,
				browserType: browser.TypeAsh,
			},
		}, {
			Name:      "ash_warn_proceeded",
			ExtraAttr: []string{"group:mainline"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: clipboardTestParams{
				name:        "warn_proceded",
				restriction: restrictionlevel.WarnProceeded,
				copyAllowed: true,
				browserType: browser.TypeAsh,
			},
		}, {
			Name:      "ash_warn_cancelled",
			ExtraAttr: []string{"group:golden_tier"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: clipboardTestParams{
				name:        "warn_cancelled",
				restriction: restrictionlevel.WarnCancelled,
				copyAllowed: false,
				browserType: browser.TypeAsh,
			},
		}, {
			Name:              "lacros_blocked",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: clipboardTestParams{
				name:        "blocked",
				restriction: restrictionlevel.Blocked,
				copyAllowed: false,
				browserType: browser.TypeLacros,
			},
		}, {
			Name:              "lacros_warn_proceeded",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: clipboardTestParams{
				name:        "warn_proceeded",
				restriction: restrictionlevel.WarnProceeded,
				copyAllowed: true,
				browserType: browser.TypeLacros,
			},
		}, {
			Name: "lacros_warn_cancelled",
			ExtraAttr: []string{
				"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: clipboardTestParams{
				name:        "warn_cancelled",
				restriction: restrictionlevel.WarnCancelled,
				copyAllowed: false,
				browserType: browser.TypeLacros,
			},
		}},
	})
}

// waitForDialog waits for a 2nd exo surface to appear. On Lacros the 'bubble' is retrievable before it is visible / clickable. This checks for a unnamed surface being visible (the browser surface is named).
func waitForDialog(ctx context.Context, s *testing.State, ui *uiauto.Context, browserType browser.Type) {
	if browserType == browser.TypeLacros {
		regex, _ := regexp.Compile("ExoShellSurface-.*")
		if err := ui.WaitUntilExists(nodewith.ClassNameRegex(regex).Name("").Visible())(ctx); err != nil {
			s.Fatal("Failed to wait for dialog: ", err)
		}
	}
}

func DataLeakPreventionRulesListClipboard(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	sourceServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer sourceServer.Close()

	destServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer destServer.Close()

	params := s.Param().(clipboardTestParams)

	path := "/text_1.html"
	// Update the policy blob.
	pb := policyBlob.NewBlob()
	switch params.restriction {
	case restrictionlevel.Blocked:
		pb.AddPolicies(policy.PopulateClipboardBlockPolicy(sourceServer.URL+path, destServer.URL))
	case restrictionlevel.WarnCancelled, restrictionlevel.WarnProceeded:
		pb.AddPolicies(policy.PopulateClipboardWarnPolicy(sourceServer.URL+path, destServer.URL))
	}

	// Update policy.
	if err := policyutil.ServeBlobAndRefresh(ctx, fakeDMS, cr, pb); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

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

	s.Log("Waiting for chrome.clipboard API to become available")
	if err := tconn.WaitForExpr(ctx, "chrome.clipboard"); err != nil {
		s.Fatal("Failed to wait for chrome.clipboard API to become available: ", err)
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, params.browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer func(ctx context.Context) {
		if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
			// The Lacros browser is not closed in other places in the test.
			s.Error("The Lacros browser probably crashed: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+params.name)

	sourceURL := sourceServer.URL + path
	sourceConn, err := br.NewConn(ctx, sourceURL)
	if err != nil {
		s.Fatalf("Failed to open page %q: %v", path, err)
	}
	defer sourceConn.Close()

	if err := webutil.WaitForQuiescence(ctx, sourceConn, 10*time.Second); err != nil {
		s.Fatalf("Failed to wait for %q to achieve quiescence: %v", path, err)
	}

	if err := uiauto.Combine("copy all text from source website",
		keyboard.AccelAction("Ctrl+A"),
		keyboard.AccelAction("Ctrl+C"))(ctx); err != nil {
		s.Fatal("Failed to copy text from source browser: ", err)
	}

	copiedString, err := clipboard.GetClipboardContent(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get clipboard content: ", err)
	}

	destURL := destServer.URL + "/editable_text_box.html"
	destConn, err := br.NewConn(ctx, destURL)
	if err != nil {
		s.Fatalf("Failed to open page %q: %v", destURL, err)
	}
	defer destConn.Close()

	if err := webutil.WaitForQuiescence(ctx, destConn, 10*time.Second); err != nil {
		s.Fatalf("Failed to wait for %q to achieve quiescence: %v", destURL, err)
	}

	ui := uiauto.New(tconn)

	textBoxNode := nodewith.Name("textarea").Role(role.TextField).State(state.Editable, true).First()
	if err := uiauto.Combine("Pasting into text box",
		ui.WaitUntilExists(textBoxNode.Visible()),
		ui.LeftClick(textBoxNode),
		ui.WaitUntilExists(textBoxNode.Focused()),
		keyboard.AccelAction("Ctrl+V"),
	)(ctx); err != nil {
		s.Fatal("Failed to paste into text box: ", err)
	}

	// Verify notification bubble.
	parsedSourceURL, _ := url.Parse(sourceServer.URL)

	switch params.restriction {
	case restrictionlevel.Blocked:
		notifError := clipboard.CheckClipboardBubble(ctx, ui, parsedSourceURL.Hostname())
		if !params.copyAllowed && notifError != nil {
			s.Error("Expected notification but found an error: ", notifError)
		}
		if params.copyAllowed && notifError == nil {
			s.Error("Didn't expect notification but one was found: ")
		}
	case restrictionlevel.WarnCancelled:
		waitForDialog(ctx, s, ui, params.browserType)
		bubbleClass, notifError := clipboard.WarnBubble(ctx, ui, parsedSourceURL.Hostname())
		if notifError != nil {
			s.Error("Expected notification but found an error: ", notifError)
		}
		cancelButton := nodewith.Name("Cancel").Role(role.Button).Ancestor(bubbleClass)
		if err := ui.LeftClick(cancelButton)(ctx); err != nil {
			s.Fatal("Failed to click the cancel button: ", err)
		}
	case restrictionlevel.WarnProceeded:
		waitForDialog(ctx, s, ui, params.browserType)
		bubbleClass, notifError := clipboard.WarnBubble(ctx, ui, parsedSourceURL.Hostname())
		if notifError != nil {
			s.Error("Expected notification but found an error: ", notifError)
		}
		pasteButton := nodewith.Name("Paste anyway").Role(role.Button).Ancestor(bubbleClass)
		if err := ui.LeftClick(pasteButton)(ctx); err != nil {
			s.Fatal("Failed to click the paste button: ", err)
		}
	}

	if params.copyAllowed {
		pastedError := clipboard.CheckPastedContent(ctx, ui, copiedString)
		if pastedError != nil {
			s.Error("Checked pasted content but found an error: ", pastedError)
		}
	} else {
		emptyError := clipboard.CheckContentIsNotPasted(ctx, ui, copiedString)
		if emptyError != nil {
			s.Error("Content was pasted but should have been blocked")
		}
	}

}
