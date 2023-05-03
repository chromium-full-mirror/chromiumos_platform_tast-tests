// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/chrome/lacros/lacrosproc"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/kioskmode"
	"go.chromium.org/tast/core/testing"
)

type ephemeralModeTestData struct {
	IsLacros    bool
	IsEphemeral bool
	Policies    []policy.Policy
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchWithDeviceEphemeralUsersEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that Kiosk configuration starts correctly with DeviceEphemeralUsersEnabled policy set to true",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"kamilszarek@google.com", // Test author
		},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.KioskAutoLaunchCleanup,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceEphemeralUsersEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Params: []testing.Param{
			{
				Name: "ash_unset",
				Val: ephemeralModeTestData{
					IsLacros:    false,
					IsEphemeral: false,
					Policies:    []policy.Policy{&policy.DeviceEphemeralUsersEnabled{Stat: policy.StatusUnset}},
				},
			},
			{
				Name: "ash_true",
				Val: ephemeralModeTestData{
					IsLacros:    false,
					IsEphemeral: true,
					Policies:    []policy.Policy{&policy.DeviceEphemeralUsersEnabled{Val: true}},
				},
			},
			{
				Name: "ash_false",
				Val: ephemeralModeTestData{
					IsLacros:    false,
					IsEphemeral: false,
					Policies:    []policy.Policy{&policy.DeviceEphemeralUsersEnabled{Val: false}},
				},
			},
			{
				Name: "lacros_unset",
				Val: ephemeralModeTestData{
					IsLacros:    true,
					IsEphemeral: false,
					Policies:    []policy.Policy{&policy.DeviceEphemeralUsersEnabled{Stat: policy.StatusUnset}},
				},
			},
			{
				Name: "lacros_true",
				Val: ephemeralModeTestData{
					IsLacros:    true,
					IsEphemeral: true,
					Policies:    []policy.Policy{&policy.DeviceEphemeralUsersEnabled{Val: true}},
				},
			},
			{
				Name: "lacros_false",
				Val: ephemeralModeTestData{
					IsLacros:    true,
					IsEphemeral: false,
					Policies:    []policy.Policy{&policy.DeviceEphemeralUsersEnabled{Val: false}},
				},
			},
		},
	})
}

func LaunchWithDeviceEphemeralUsersEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(ephemeralModeTestData)

	opts := []kioskmode.Option{
		kioskmode.DefaultLocalAccounts(),
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
		kioskmode.ExtraPolicies(param.Policies),
	}
	if param.IsLacros {
		opts = append(opts, kioskmode.PublicAccountPolicies(kioskmode.KioskAppAccountID,
			[]policy.Policy{&policy.LacrosAvailability{Val: "lacros_only"}}))
	}

	kiosk, cr, err := kioskmode.DeprecatedNew(ctx, fdms, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.DeprecatedClose(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(ctx)

	testing.ContextLog(ctx, "Checking the mount type of the Kiosk cryptohome (permanent or ephemeral)")
	expectedMountType := cryptohome.Permanent
	if param.IsEphemeral {
		expectedMountType = cryptohome.Ephemeral
	}
	userID := kioskmode.DeviceLocalAccountUserID(&kioskmode.KioskAppAccountInfo)
	if err := cryptohome.WaitForUserMountAndValidateType(ctx, userID, expectedMountType); err != nil {
		s.Fatal("Failed to wait for user mount and validate type: : ", err)
	}

	if param.IsLacros {
		testing.ContextLog(ctx, "Checking if Kiosk started in Lacros mode")
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to create Test API connection: ", err)
		}
		if _, err = lacrosproc.Root(ctx, tconn); err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}
}
