// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           CarrierLockEndToEndWildcard,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that cellular can connect to any carrier with wildcard profile",
		Contacts:       []string{"ujjwalpande@google.com", "chromeos-cellular-team@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:        "cellularSIMLockCleared",
		SoftwareDeps:   []string{"chrome"},
		Timeout:        20 * time.Minute,
		VarDeps:        []string{"cellular.gaiaAccountPool"},
		Params: []testing.Param{
			{
				Name:              "",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("yavilla")),
			},
		},
	})
}

// CarrierLockEndToEndWildcard validates that all carriers can connect with wild card
// carrier lock config.
func CarrierLockEndToEndWildcard(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	helper.PrintSIMInfo(ctx)

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID")
	}

	carrier, err := helper.GetCarrierNameForICCID(ctx, iccid)
	if err != nil {
		s.Fatal("Could not get carrier name")
	}

	s.Log("ICCID: ", iccid, " carrier: ", carrier)

	// Ensure that a Cellular Service was created.
	if _, err := helper.FindService(ctx); err != nil {
		s.Fatal("Unable to find Cellular Service: ", err)
	}

	_, err = modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	gaiaCreds, err := credconfig.PickRandomCreds(s.RequiredVar("cellular.gaiaAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse cellular user creds: ", err)
	}

	s.Log("Create and upload CSV file to lock the device")
	// Create and upload appropriate lock file for wild card config
	err = helper.CreateAndUploadCarrierLockCsv(ctx, gaiaCreds, cellular.SimLockWildcardProfileID)
	if err != nil {
		s.Fatal("Failed to create and upload CSV file: ", err)
	}

	// Try to unlock the device in the SimLock portal even if the test bails out due to errors,
	// to avoid leaving the DUT in a locked state which will impact subsequent tests
	// running on this DUT.
	defer helper.CarrierUnlockDevice(cleanupCtx, gaiaCreds)

	s.Log("Wait for SimLock info to be propagated to the PSM")
	// GoBigSleepLint: Wait for SimLock info to be propagated to the PSM.
	// Currently there is no way to know if information synced to PSM server other
	// than to wait for predefined time. This can be tuned later based
	// on average time to sync.
	// Keeping this to 5 minute based on the guidance from PSM team.
	testing.Sleep(ctx, 5*time.Minute)

	uiHelper, err := cellular.NewUIHelper(ctx, gaiaCreds.User, gaiaCreds.Pass)
	if err != nil {
		s.Fatal("Failed to create cellular.NewUiHelper: ", err)
	}
	defer faillog.DumpUITree(ctx, s.OutDir(), uiHelper.Tconn)
	uiHelper.LaunchChromeWithCarrierLock(ctx, gaiaCreds.User, gaiaCreds.Pass)

	s.Log("Wait for sim lock type to change to none")
	if err = helper.WaitForCarrierLock(ctx, false); err != nil {
		s.Fatal("Card not reported as unlocked with wild card config: ", carrier)
	}

	s.Log("upload the unlock config")
	// Create and upload appropriate unlock file
	err = helper.CreateAndUploadCarrierLockCsv(ctx, gaiaCreds, cellular.SimLockUnlockProfileID)
	if err != nil {
		s.Fatal("Failed to create and upload CSV file: ", err)
	}

	s.Log("Wait for sim lock type to change to none")
	if err = helper.WaitForCarrierLock(ctx, false); err != nil {
		s.Fatal("Card not reported as unlocked after unlock in portal: ", carrier)
	}
}
