// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	pmpb "chromiumos/system_api/power_manager_proto"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeOsLockOnIdleSuspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Behavior of ChromeOsLockOnIdleSuspend policy, checking the correspoding toggle button states (restriction and checked) and the lock screen after the lid is closed",
		Contacts: []string{
			"cros-lurs@google.com",
			"ultrotter@google.com",
			"antrim@google.com",
			"emaxx@google.com",
		},
		BugComponent: "b:1277523",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ChromeOsLockOnIdleSuspend{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.ChromeOsLockOnIdleSuspend{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// 1. Configure an OU with a user policy A set to X
				// 2. Log in with the managed account
				// 3. Ensure that the policy A is set to X on the device
				// COM_FOUND_CUJ30_TASK2_WF1
				Value: "screenplay-80b20d5f-c733-45b8-8bdd-f876457a5145",
			},
		},
	})
}

// ChromeOsLockOnIdleSuspend tests the ChromeOsLockOnIdleSuspend policy.
func ChromeOsLockOnIdleSuspend(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	emitter, err := power.NewPowerManagerEmitter(ctx)
	if err != nil {
		s.Fatal("Unable to create power manager emitter: ", err)
	}
	defer func(ctx context.Context) {
		if err := emitter.Stop(ctx); err != nil {
			s.Log("Unable to stop emitter: ", err)
		}
	}(cleanupCtx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed creating keyboard: ", err)
	}
	defer kb.Close(ctx)

	const lockTimeout = 5 * time.Second

	for _, param := range []struct {
		name            string
		wantLockDevice  bool                              // wantLockDevice is the wanted lock state of the device after the lid is closed.
		wantRestriction restriction.Restriction           // wantRestriction is the wanted restriction state of the checkboxes in Browsing history.
		wantChecked     checked.Checked                   // wantChecked is the wanted checked state of the checkboxes in Browsing history.
		policyValue     *policy.ChromeOsLockOnIdleSuspend // policyValue is the value of the ChromeOsLockOnIdleSuspend policy.
	}{
		{
			name:            "forced",
			wantLockDevice:  true,
			wantRestriction: restriction.Disabled,
			wantChecked:     checked.True,
			policyValue:     &policy.ChromeOsLockOnIdleSuspend{Val: true},
		},
		{
			name:            "disabled",
			wantLockDevice:  false,
			wantRestriction: restriction.Disabled,
			wantChecked:     checked.False,
			policyValue:     &policy.ChromeOsLockOnIdleSuspend{Val: false},
		},
		{
			name:            "unset",
			wantLockDevice:  false,
			wantRestriction: restriction.None,
			wantChecked:     checked.False,
			policyValue:     &policy.ChromeOsLockOnIdleSuspend{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policyValue}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Open the Security and sign-in page where the affected toggle button can be found.
			if err := policyutil.OSSettingsPageWithPassword(ctx, cr, "osPrivacy/lockScreen", fixtures.Password).
				SelectNode(ctx, nodewith.
					Role(role.ToggleButton).
					Name("Lock when sleeping or lid is closed")).
				Restriction(param.wantRestriction).
				Checked(param.wantChecked).
				Verify(); err != nil {
				s.Error("Unexpected OS settings state: ", err)
			}

			// Close the lid and check if the policy works correctly.
			// TODO(b/257211713): check if the policy works correctly when the user is idle and the device suspends.
			eventType := pmpb.InputEvent_LID_CLOSED
			if err := emitter.EmitInputEvent(ctx, &pmpb.InputEvent{Type: &eventType}); err != nil {
				s.Fatal("Sending LID_CLOSED failed: ", err)
			}
			// Defer the screen unlock to ensure subsequent tests aren't affected by the screen remaining locked.
			defer func(ctx context.Context) {
				eventType := pmpb.InputEvent_LID_OPEN
				if err := emitter.EmitInputEvent(ctx, &pmpb.InputEvent{Type: &eventType}); err != nil {
					s.Fatal("Sending LID_OPEN failed: ", err)
				}

				const authTimeout = 5 * time.Second

				st, _ := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked }, lockTimeout)

				if st.Locked {
					s.Log("Unlocking screen by typing the given password")

					if err := lockscreen.UnlockWithPassword(ctx, tconn, fixtures.Username, fixtures.Password, kb, lockTimeout, authTimeout); err != nil {
						s.Fatal("Failed to unlock the screen: ", err)
					}
				}
			}(cleanupCtx)

			st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, lockTimeout)
			screenLocked := st.Locked && st.ReadyForPassword

			if screenLocked && !param.wantLockDevice {
				s.Fatal("Screen should not be locked: ", err)
			}

			if !screenLocked && param.wantLockDevice {
				s.Fatal("Screen should be locked: ", err)
			}
		})
	}
}
