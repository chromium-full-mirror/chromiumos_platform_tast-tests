// Copyright 2021 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListClipboardShelf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with clipboard blocked restriction in the shelf textfield",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"ayaelattar@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:hw_agnostic"},
		Data:         []string{"text_1.html", "text_2.html"},
		Params: []testing.Param{{
			Name:      "ash",
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val:       browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraAttr:         []string{"group:mainline", "informational", "group:criticalstaging"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
	})
}

func DataLeakPreventionRulesListClipboardShelf(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	allowedServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer allowedServer.Close()

	blockedServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer blockedServer.Close()

	// Set DLP policy with all clipboard blocked restriction.
	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, policy.RestrictiveDLPPolicyForClipboard(blockedServer.URL)); err != nil {
		s.Fatal("Failed to serve and verify: ", err)
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

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	for _, param := range []struct {
		name        string
		wantAllowed bool
		sourceURL   string
	}{
		{
			name:        "wantDisallowed",
			wantAllowed: false,
			sourceURL:   blockedServer.URL + "/text_1.html",
		},
		{
			name:        "wantAllowed",
			wantAllowed: true,
			sourceURL:   allowedServer.URL + "/text_2.html",
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			if err := cr.ResetState(ctx); err != nil {
				s.Fatal("Failed to reset the Chrome: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer func(ctx context.Context) {
				if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
					// The Lacros browser is not closed in other places in the test.
					s.Error("The Lacros browser probably crashed: ", err)
				}
			}(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			conn, err := br.NewConn(ctx, param.sourceURL)
			if err != nil {
				s.Fatal("Failed to open page: ", err)
			}
			defer conn.Close()

			if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
				s.Fatalf("Failed to wait for %q to be loaded and achieve quiescence: %s", param.sourceURL, err)
			}

			if err := uiauto.Combine("copy all text from source website",
				keyboard.AccelAction("Ctrl+A"),
				keyboard.AccelAction("Ctrl+C"))(ctx); err != nil {
				s.Fatal("Failed to copy text from source browser: ", err)
			}

			// Open the launcher.
			if err := launcher.Open(tconn)(ctx); err != nil {
				s.Fatal("Failed to open the launcher: ", err)
			}

			if err := launcher.WaitForStableNumberOfApps(ctx, tconn); err != nil {
				s.Fatal("Failed to wait for item count in app list to stabilize: ", err)
			}

			parsedSourceURL, err := url.Parse(blockedServer.URL)
			if err != nil {
				s.Fatal("Failed to parse blocked server url: ", err)
			}

			s.Log("Pasting content in shelf box")
			if err := pasteShelfbox(ctx, tconn, keyboard, parsedSourceURL.Hostname(), param.wantAllowed); err != nil {
				s.Error("Failed to paste content in shelf box: ", err)
			}
		})
	}
}

func pasteShelfbox(ctx context.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, url string, wantAllowed bool) error {
	ui := uiauto.New(tconn)

	if err := keyboard.AccelAction("Ctrl+V")(ctx); err != nil {
		return errors.Wrap(err, "failed to paste content in shelf box")
	}

	err := clipboard.CheckClipboardBubble(ctx, ui, url)
	if err != nil && !wantAllowed {
		return err
	}

	if err == nil && wantAllowed {
		return errors.New("Notification found, expected none")
	}

	return nil
}
