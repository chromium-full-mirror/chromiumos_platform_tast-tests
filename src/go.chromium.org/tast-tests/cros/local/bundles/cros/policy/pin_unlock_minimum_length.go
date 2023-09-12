// Copyright 2020 The ChromiumOS Authors
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
		Func:         PinUnlockMinimumLength,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Follows the user flow to set unlock pin",
		Contacts: []string{
			"cros-lurs@google.com",
			"mbid@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChromePolicyLoggedInLockscreen,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.QuickUnlockModeAllowlist{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.PinUnlockMinimumLength{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func PinUnlockMinimumLength(ctx context.Context, s *testing.State) {
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

	for _, param := range []struct {
		name     string
		pin      string
		warning  string
		policies []policy.Policy
	}{
		{
			name:    "unset",
			pin:     "135246",
			warning: "PIN must be at least 6 digits",
			policies: []policy.Policy{
				&policy.PinUnlockMinimumLength{Stat: policy.StatusUnset},
				&policy.QuickUnlockModeAllowlist{Val: []string{"PIN"}},
			},
		},
		{
			name:    "shorter",
			pin:     "1342",
			warning: "PIN must be at least 4 digits",
			policies: []policy.Policy{
				&policy.PinUnlockMinimumLength{Val: 4},
				&policy.QuickUnlockModeAllowlist{Val: []string{"PIN"}},
			},
		},
		{
			name:    "longer",
			pin:     "13574268",
			warning: "PIN must be at least 8 digits",
			policies: []policy.Policy{
				&policy.PinUnlockMinimumLength{Val: 8},
				&policy.QuickUnlockModeAllowlist{Val: []string{"PIN"}},
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
				// Entering a good PIN will make Continue button clickable and the warning message will disappear.
				SetUpPin(ctx, param.pin, false /*completeSetup*/).
				Verify(); err != nil {
				s.Fatal("Failed to setup pin: ", err)
			}

			ui := uiauto.New(tconn)

			continueButton := nodewith.Name("Continue").Role(role.Button)
			warningMessage := nodewith.Name(param.warning).Role(role.StaticText)

			// Find the Continue button node.
			if err := ui.WaitUntilExists(continueButton)(ctx); err != nil {
				s.Fatal("Could not find the Continue button: ", err)
			}

			// Wait for the warning message to disappear.
			if err := ui.WaitUntilGone(warningMessage)(ctx); err != nil {
				s.Fatal("The warning message isn't gone yet: ", err)
			}

			// Find the node info for the Continue button.
			nodeInfo, err := ui.Info(ctx, continueButton)
			if err != nil {
				s.Fatal("Could not get info for the Continue button: ", err)
			}

			// Check Continue button state, it must be clickable with a good pin.
			if nodeInfo.Restriction == restriction.Disabled {
				s.Error("The continue button is disabled while the pin is good")
			}

			// Press backspace to remove 1 digit to get a bad pin.
			if err := kb.Accel(ctx, "Backspace"); err != nil {
				s.Fatal("Failed to press backspace: ", err)
			}

			// Wait for the warning message to appear again after removing 1 digit.
			if err := ui.WaitUntilExists(warningMessage)(ctx); err != nil {
				s.Fatal("Failed to find the warning message (2): ", err)
			}

			nodeInfo, err = ui.Info(ctx, continueButton)
			if err != nil {
				s.Fatal("Could not get new info for the Continue button: ", err)
			}
			// Check Continue button state, it must be disabled with a bad pin.
			if nodeInfo.Restriction != restriction.Disabled {
				s.Fatal("Continue button should be disabled")
			}
		})
	}
}
