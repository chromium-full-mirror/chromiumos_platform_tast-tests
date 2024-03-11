// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PinUnlockMaximumLength,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the maximum length of the unlock PIN",
		Contacts: []string{
			"cros-lurs@google.com",
			"emaamari@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChromePolicyLoggedInLockscreen,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.QuickUnlockModeAllowlist{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.PinUnlockMaximumLength{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func PinUnlockMaximumLength(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	quickUnlockModeAllowlistPolicy := &policy.QuickUnlockModeAllowlist{Val: []string{"PIN"}}

	for _, param := range []struct {
		name     string
		pin      string
		warning  string
		policies []policy.Policy
	}{
		{
			name:    "unset",
			pin:     "13574268135742681357426813574268",
			warning: "",
			policies: []policy.Policy{
				&policy.PinUnlockMaximumLength{Stat: policy.StatusUnset},
				quickUnlockModeAllowlistPolicy,
			},
		},
		{
			name:    "set",
			pin:     "13421342134",
			warning: "PIN must be less than 11 digits",
			policies: []policy.Policy{
				&policy.PinUnlockMaximumLength{Val: 10},
				quickUnlockModeAllowlistPolicy,
			},
		},
		{
			name:    "smaller-than-min",
			pin:     "1357426",
			warning: "PIN must be less than 7 digits",
			policies: []policy.Policy{
				&policy.PinUnlockMaximumLength{Val: 4},
				quickUnlockModeAllowlistPolicy,
			},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Open the Lockscreen page and enter a PIN.
			if err := policyutil.OSSettingsPageWithPassword(ctx, cr, "osPrivacy/lockScreen", fixtures.Password).
				SetUpPin(ctx, param.pin, false /*completeSetup*/).
				Verify(); err != nil {
				s.Fatal("Failed to setup pin: ", err)
			}

			ui := uiauto.New(tconn)
			continueButton := nodewith.Name("Continue").Role(role.Button)

			// Find the Continue button node.
			if err := ui.WaitUntilExists(continueButton)(ctx); err != nil {
				s.Fatal("Could not find the Continue button: ", err)
			}

			if len(param.warning) == 0 {
				// If no warning is expected, check that the Continue button is enabled.
				if err := ui.WaitUntilEnabled(continueButton)(ctx); err != nil {
					s.Error("Continue button should be enabled")
				}
			} else {
				// If there is a warning message, check that it is displayed.
				if err := ui.WaitUntilExists(nodewith.Name(param.warning).Role(role.StaticText))(ctx); err != nil {
					s.Fatal("Failed to find the warning message: ", err)
				}

				// Also check that Continue button is disabled.
				if err := ui.WaitForRestriction(continueButton, restriction.Disabled)(ctx); err != nil {
					s.Fatal("Failed to wait for continueButton disabled: ", err)
				}

				// Press backspace to remove 1 digit to get a good PIN.
				if err := kb.Accel(ctx, "Backspace"); err != nil {
					s.Fatal("Failed to press backspace: ", err)
				}

				// After removing a digit the warning should be disappear.
				if err := ui.WaitUntilGone(nodewith.Name(param.warning).Role(role.StaticText))(ctx); err != nil {
					s.Fatal("Failed to wait the warning message is gone: ", err)
				}

				// Wait until the Continue button is enabled again.
				if err := ui.WaitUntilEnabled(continueButton)(ctx); err != nil {
					s.Error("Continue button should be enabled")
				}

			}
		})
	}
}
