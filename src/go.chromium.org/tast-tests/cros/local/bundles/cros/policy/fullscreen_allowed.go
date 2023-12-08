// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FullscreenAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of FullscreenAllowed policy: checking if fullscreen is allowed or not",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"swapnilgupta@google.com", // Test author
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.FullscreenAllowed{}, pci.VerifiedFunctionalityJS),
		},
	})
}

// FullscreenAllowed tests the FullscreenAllowed policy.
func FullscreenAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	for _, param := range []struct {
		name                  string
		value                 *policy.FullscreenAllowed
		wantFullscreenEnabled bool
	}{
		{
			name:                  "unset",
			value:                 &policy.FullscreenAllowed{Stat: policy.StatusUnset},
			wantFullscreenEnabled: true,
		},
		{
			name:                  "disabled",
			value:                 &policy.FullscreenAllowed{Val: false},
			wantFullscreenEnabled: false,
		},
		{
			name:                  "enabled",
			value:                 &policy.FullscreenAllowed{Val: true},
			wantFullscreenEnabled: true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), "about:blank")
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)
			defer conn.Close()
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			var isFullScreen bool
			if err := conn.Eval(ctx, `window.innerHeight == screen.availHeight`, &isFullScreen); err != nil {
				s.Fatal("Failed to execute JS expression: ", err)
			}
			// Check that the screen is not in full screen mode currently.
			if isFullScreen {
				s.Error("Browser should be not be in full screen mode initially")
			}

			// Define keyboard to type keyboard shortcut.
			// On physical keyboards the "fullscreen" hotkey needs to be pressed instead of F11 most of the times.
			// On tablet devices only virtual keyboards are available.
			// Using a virtual keyboard on all boards is better to enter the full screen mode.
			kb, err := input.VirtualKeyboard(ctx)
			if err != nil {
				s.Fatal("Failed to get the keyboard: ", err)
			}
			defer kb.Close(ctx)

			// Press the fullscreen hotkey to enter full screen mode.
			if err := kb.Accel(ctx, "f11"); err != nil {
				s.Fatal("Failed to type fullscreen hotkey: ", err)
			}

			// Wait for full screen to happen.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				// Check whether the browser is currently in full screen mode.
				if err := conn.Eval(ctx, `window.innerHeight == screen.availHeight`, &isFullScreen); err != nil {
					return errors.Wrap(err, "failed to execute JS expression")
				}
				if isFullScreen != param.wantFullscreenEnabled {
					return errors.Errorf("unexpected full screen state: got %v, want %v", isFullScreen, param.wantFullscreenEnabled)
				}
				return nil
			}, &testing.PollOptions{
				Timeout:  5 * time.Second,
				Interval: 1 * time.Second,
			}); err != nil {
				s.Error("Polling for having full screen failed: ", err)
			}
		})
	}
}
