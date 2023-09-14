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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParam struct {
	fingerprintSupported bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         QuickUnlockModeAllowlist,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that quick unlock options are enabled or disabled based on the policy value",
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
		Params: []testing.Param{
			{
				Val: testParam{fingerprintSupported: false},
			},
			{
				Name:              "fingerprint_test",
				ExtraHardwareDeps: hwdep.D(hwdep.Fingerprint()),
				Val:               testParam{fingerprintSupported: true},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.QuickUnlockModeAllowlist{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.WebAuthnFactors{}, pci.VerifiedValue),
		},
	})
}

// QuickUnlockModeAllowlist sets up multiple policies, but only tests behavior that it controls.
// It tests "setup" and "quick_unlock", but not "webauthn" or other auth usages. So it will include
// just enough test cases to verify:
// 1. QuickUnlockModeAllowlist enabled will enable "setup" the auth method and using it for "quick_unlock",
//
//	even if all other policies disable it.
//
// 2. QuickUnlockModeAllowlist disabled will disable using the auth method for "quick_unlock" even if all other
//
//	policies enabled it, but will not disable "setup" for that auth method.
func QuickUnlockModeAllowlist(ctx context.Context, s *testing.State) {
	type testCase struct {
		name                     string
		quickUnlockModeAllowlist policy.QuickUnlockModeAllowlist
		// Since this policy have similar set of entries and controls whether an auth method can be set with
		// QuickUnlockModeAllowlist together, we want test cases that verify behaviors are correct when both
		// policies are set and have different values. See comments in quickUnlockTestCases.
		webAuthnFactors policy.WebAuthnFactors
	}

	const PIN = "123456"

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

	testCases := []testCase{
		{
			name:                     "unset",
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Stat: policy.StatusUnset},
			webAuthnFactors:          policy.WebAuthnFactors{Stat: policy.StatusUnset},
		},
		{
			name:                     "empty",
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Val: []string{}},
			webAuthnFactors:          policy.WebAuthnFactors{Stat: policy.StatusUnset},
		},
		// WebAuthnFactors set to empty list shouldn't affect set and unlock capabilities.
		{
			name:                     "all",
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Val: []string{"all"}},
			webAuthnFactors:          policy.WebAuthnFactors{Val: []string{}},
		},
		{
			name:                     "pin",
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Val: []string{"PIN"}},
			webAuthnFactors:          policy.WebAuthnFactors{Stat: policy.StatusUnset},
		},
		// WebAuthnFactors set to all will allow user to set up PIN, but shouldn't allow user
		// to unlock screen using PIN.
		{
			name:                     "quick_unlock_empty_webauthn_all",
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Val: []string{}},
			webAuthnFactors:          policy.WebAuthnFactors{Val: []string{"all"}},
		},
	}

	fingerprintSupported := s.Param().(testParam).fingerprintSupported

	if fingerprintSupported {
		testCases = append(testCases, testCase{
			name:                     "fingerprint",
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Val: []string{"FINGERPRINT"}},
			webAuthnFactors:          policy.WebAuthnFactors{Stat: policy.StatusUnset},
		},
		)
	}

	for _, param := range testCases {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeOnErrorToFile(ctx, s.OutDir(), s.HasError, tconn, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			policies := []policy.Policy{
				&param.quickUnlockModeAllowlist,
				&param.webAuthnFactors,
			}

			// Update policies.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Open the Lockscreen page where we can set a PIN.
			settingsPage := policyutil.OSSettingsPageWithPassword(ctx, cr, "osPrivacy/lockScreen", fixtures.Password)
			ui := uiauto.New(tconn)

			if fingerprintSupported {
				fingerprintCapabilities := getExpectedQuickUnlockCapabilities(&param.quickUnlockModeAllowlist, &param.webAuthnFactors, "FINGERPRINT")
				found, err := ui.IsNodeFound(ctx, nodewith.Name("Edit Fingerprints").Role(role.StaticText))
				if err != nil {
					s.Fatal("Failed to find Edit Fingerprints node: ", err)
				}
				if found != fingerprintCapabilities.set {
					s.Errorf("Failed checking if fingerprint can be set: got %v, want %v", found, fingerprintCapabilities.set)
				}
			}

			pinCapabilities := getExpectedQuickUnlockCapabilities(&param.quickUnlockModeAllowlist, &param.webAuthnFactors, "PIN")
			var wantRestriction restriction.Restriction
			if pinCapabilities.set {
				wantRestriction = restriction.None
			} else {
				wantRestriction = restriction.Disabled
			}

			// Check that the PIN button has the expected restriction.
			if err := settingsPage.
				SelectNode(ctx, nodewith.Name("Set up").Role(role.Button)).
				Restriction(wantRestriction).
				Verify(); err != nil {
				s.Fatal("Unexpected button state: want ", wantRestriction)
			}

			// If PIN can be set, we set up a PIN and see if the lock screen UI corresponds to PIN's unlock capability.
			if pinCapabilities.set {
				if err := settingsPage.
					SetUpPin(ctx, PIN, true /*completeSetup*/).
					Verify(); err != nil {
					s.Fatal("Unexpected button state: want ", wantRestriction)
				}

				if err := lockAndUnlockScreen(ctx, tconn, kb, fixtures.Password, PIN, pinCapabilities.unlock); err != nil {
					s.Fatal("Failed to lock and unlock the screen using PIN or password: ", err)
				}

				if err := uiauto.Combine("delete PIN",
					ui.LeftClick(nodewith.HasClass("icon-more-vert").Ancestor(ossettings.WindowFinder).Role(role.PopUpButton)),
					ui.LeftClick(nodewith.Name("Remove").Ancestor(ossettings.WindowFinder).Role(role.MenuItem)))(ctx); err != nil {
					s.Fatal("Failed to delete PIN: ", err)
				}
			}
		})
	}
}

type quickUnlockCapabilities struct {
	// Whether the auth method is allowed to be set in OS Settings.
	set bool
	// Whether the auth method is allowed to be used for unlocking the screen.
	unlock bool
}

func getExpectedQuickUnlockCapabilities(quickUnlockModeAllowlist *policy.QuickUnlockModeAllowlist, webauthnFactors *policy.WebAuthnFactors, authMethod string) quickUnlockCapabilities {
	set, unlock := false, false
	if quickUnlockModeAllowlist.Stat != policy.StatusUnset {
		for _, entry := range quickUnlockModeAllowlist.Val {
			if entry == authMethod || entry == "all" {
				set = true
				unlock = true
			}
		}
	}
	if webauthnFactors.Stat != policy.StatusUnset {
		for _, entry := range webauthnFactors.Val {
			if entry == authMethod || entry == "all" {
				set = true
			}
		}
	}
	return quickUnlockCapabilities{
		set,
		unlock,
	}
}

func lockAndUnlockScreen(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, password, PIN string, pinEnabled bool) error {
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to lock the screen")
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
		return errors.Wrapf(err, "waiting for screen to be locked failed (last status %+v)", st)
	}

	if pinEnabled {
		if err := lockscreen.EnterPIN(ctx, tconn, kb, PIN); err != nil {
			return errors.Wrap(err, "failed to enter in PIN")
		}

		if err := lockscreen.SubmitPINOrPassword(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to submit PIN")
		}
	} else {
		if err := kb.Type(ctx, password+"\n"); err != nil {
			return errors.Wrap(err, "failed to enter password")
		}
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return !st.Locked }, 30*time.Second); err != nil {
		return errors.Wrapf(err, "waiting for screen to be unlocked failed (last status %+v)", st)
	}
	return nil
}
