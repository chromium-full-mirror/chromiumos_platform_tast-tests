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
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type vkTestCase struct {
	name          string          // name is the subtest name.
	wantedAllowVK bool            // wantedAllowVK describes if virtual keyboard is expected to be shown or not.
	policies      []policy.Policy // policies is the policies values.
}

func init() {
	testing.AddTest(&testing.Test{
		Func: VirtualKeyboard,
		Desc: "Behavior of VirtualKeyboardEnabled and TouchVirtualKeyboardEnabled policies and their mixing by checking that the virtual keyboard (is/is not) displayed as requested by the policy",
		Contacts: []string{
			"e14s-eng@google.com",
		},
		BugComponent: "b:95887",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Params: []testing.Param{
			{
				Name:    "both",
				Fixture: fixture.ChromePolicyLoggedIn,
				Val: []vkTestCase{
					{
						name:          "vke_enabled-tvke_enabled",
						wantedAllowVK: true,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Val: true}, &policy.TouchVirtualKeyboardEnabled{Val: true}},
					},
					{
						name:          "vke_enabled-tvke_disabled",
						wantedAllowVK: true,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Val: true}, &policy.TouchVirtualKeyboardEnabled{Val: false}},
					},
					{
						name:          "vke_disabled-tvke_enabled",
						wantedAllowVK: true,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Val: false}, &policy.TouchVirtualKeyboardEnabled{Val: true}},
					},
					{
						name:          "vke_disabled-tvke_disabled",
						wantedAllowVK: false,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Val: false}, &policy.TouchVirtualKeyboardEnabled{Val: false}},
					},
					{
						name:          "vke_unset-tvke_enabled",
						wantedAllowVK: true,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Stat: policy.StatusUnset}, &policy.TouchVirtualKeyboardEnabled{Val: true}},
					},
					{
						name:          "vke_enabled-tvke_unset",
						wantedAllowVK: true,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Val: true}, &policy.TouchVirtualKeyboardEnabled{Stat: policy.StatusUnset}},
					},
					{
						name:          "vke_unset-tvke_disabled",
						wantedAllowVK: false,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Stat: policy.StatusUnset}, &policy.TouchVirtualKeyboardEnabled{Val: false}},
					},
					{
						name:          "vke_disabled-tvke_unset",
						wantedAllowVK: false,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Val: false}, &policy.TouchVirtualKeyboardEnabled{Stat: policy.StatusUnset}},
					},
					{
						name:          "vke_unset-tvke_unset",
						wantedAllowVK: false,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Stat: policy.StatusUnset}, &policy.TouchVirtualKeyboardEnabled{Stat: policy.StatusUnset}},
					},
				},
			},
			{
				Name:    "virtual",
				Fixture: fixture.ChromePolicyLoggedIn,
				Val: []vkTestCase{
					{
						name:          "enabled",
						wantedAllowVK: true,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Val: true}},
					},
					{
						name:          "disabled",
						wantedAllowVK: false,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Val: false}},
					},
					{
						name:          "unset",
						wantedAllowVK: false,
						policies:      []policy.Policy{&policy.VirtualKeyboardEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					{
						Key: "feature_id",
						// Enable on-screen keyboard.
						Value: "screenplay-3daf683c-be30-4cd9-a94c-276e6ad05346",
					}},
			},
			{
				Name:    "touch_virtual",
				Fixture: fixture.ChromePolicyLoggedIn,
				Val: []vkTestCase{
					{
						name:          "enabled",
						wantedAllowVK: true,
						policies:      []policy.Policy{&policy.TouchVirtualKeyboardEnabled{Val: true}},
					},
					{
						name:          "disabled",
						wantedAllowVK: false,
						policies:      []policy.Policy{&policy.TouchVirtualKeyboardEnabled{Val: false}},
					},
					{
						name:          "unset",
						wantedAllowVK: false,
						policies:      []policy.Policy{&policy.TouchVirtualKeyboardEnabled{Stat: policy.StatusUnset}},
					},
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.VirtualKeyboardEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.TouchVirtualKeyboardEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// VirtualKeyboard applies VK related policies and uses browser's address bar
// to bring the virtual keyboard up. Then asserts according to expectations.
func VirtualKeyboard(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	uia := uiauto.New(tconn)

	tcs := s.Param().([]vkTestCase)

	for _, tc := range tcs {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			// Get tablet mode state.
			tabletModeEnabled, err := ash.TabletModeEnabled(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get tablet mode state: ", err)
			}

			// Restore the tablet mode to the initial state.
			defer ash.SetTabletModeEnabled(ctx, tconn, tabletModeEnabled)

			// Set tablet mode to false - turn DUT to desktop mode.
			if err := ash.SetTabletModeEnabled(ctx, tconn, false); err != nil {
				s.Fatal("Failed to set tablet mode enabled to false: ", err)
			}

			// Reset Chrome.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to reset Chrome: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, tc.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(
				ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+tc.name)

			conn, err := cr.NewConn(ctx, "")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}
			defer conn.Close()

			// Click the address bar.
			if err := uia.LeftClick(browserui.AddressBarFinder)(ctx); err != nil {
				s.Fatal("Failed to click address bar: ", err)
			}
			vkNode := nodewith.Name("Chrome OS Virtual Keyboard").Role(role.Keyboard)
			if tc.wantedAllowVK {
				// Confirm that the virtual keyboard exists.
				if err := uia.WaitUntilExists(vkNode)(ctx); err != nil {
					s.Errorf("Virtual keyboard did not show up: %s", err)
				}
			} else {
				// Confirm that the virtual keyboard does not exist.
				if err := uia.EnsureGoneFor(vkNode, 15*time.Second)(ctx); err != nil {
					s.Error("Virtual keyboard has shown")
				}
			}
		})
	}
}
