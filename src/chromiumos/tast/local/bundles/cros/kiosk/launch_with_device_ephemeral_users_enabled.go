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
	"chromiumos/tast/local/kioskmode"
	"chromiumos/tast/testing"
)

type testData struct {
	policies []policy.Policy
	isLacros bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchWithDeviceEphemeralUsersEnabled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Kiosk configuration starts correctly with DeviceEphemeralUsersEnabled policy set to true",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"kamilszarek@google.com", // Test author
		},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.KioskAutoLaunchCleanup,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceEphemeralUsersEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Params: []testing.Param{
			{
				Name: "ash",
				Val: testData{
					isLacros: false,
				},
			},
			{
				Name: "lacros",
				Val: testData{
					isLacros: true,
					policies: []policy.Policy{
						&policy.LacrosAvailability{Val: "lacros_only"},
					},
				},
			},
		},
	})
}

func LaunchWithDeviceEphemeralUsersEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(testData)
	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		kioskmode.DefaultLocalAccounts(),
		// https://crbug.com/1202902 combining DeviceEphemeralUsersEnabled
		// with Kiosk autolaunch caused Kiosk not starting successfully.
		kioskmode.ExtraPolicies([]policy.Policy{&policy.DeviceEphemeralUsersEnabled{Val: true}}),
		kioskmode.PublicAccountPolicies(kioskmode.KioskAppAccountID, param.policies),
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
	)
	if err != nil {
		s.Error("Failed to start Chrome in Kiosk mode: ", err)
	}

	defer kiosk.Close(ctx)

	if param.isLacros {
		testing.ContextLog(ctx, "Checking if Kiosk started in Lacros mode")
		testConn, err := cr.TestAPIConn(ctx)
		_, err = lacrosproc.Root(ctx, testConn)
		if err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}
}
