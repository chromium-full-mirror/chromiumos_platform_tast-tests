// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast/core/ctxutil"
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
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      kioskmode.SetupDuration + 2*kioskmode.LaunchDuration + kioskmode.CleanupDuration,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceEphemeralUsersEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
		},
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
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
		kioskmode.ExtraPolicies(param.Policies),
	}
	if param.IsLacros {
		opts = append(opts, kioskmode.PublicAccountPolicies(
			kioskmode.KioskAppAccountID,
			&policy.LacrosAvailability{Val: "lacros_only"},
		))
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, cr, err := kioskmode.New(ctx, fdms, s.RequiredVar("ui.signinProfileTestExtensionManifestKey"), opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close Kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

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
