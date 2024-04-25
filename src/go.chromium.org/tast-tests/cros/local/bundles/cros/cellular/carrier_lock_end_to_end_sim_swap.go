// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	cellularcommon "go.chromium.org/tast-tests/cros/common/cellular"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/starfish"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           CarrierLockEndToEndSIMSwap,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Validates sim swap on carrier locked device",
		Contacts:       []string{"chromeos-cellular-team@google.com", "ujjwalpande@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{},
		Fixture:        "starfish",
		SoftwareDeps:   []string{"chrome"},
		Timeout:        20 * time.Minute,
		VarDeps:        []string{"cellular.gaiaAccountPool"},
		Vars:           []string{"autotest_host_info_labels"},
		Params: []testing.Param{
			{
				Name:              "",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("yavilla")),
			},
		},
	})
}

// CarrierLockEndToEndSIMSwap validates sim swap on carrier locked device.
func CarrierLockEndToEndSIMSwap(ctx context.Context, s *testing.State) {
	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Could not create cellular helper: ", err)
	}

	var dutInfo *cellularcommon.DUTInfo
	if dutConfig, err := s.ChromeOSDUTLabConfig(""); err == nil {
		dutInfo = cellularcommon.NewDUTInfoFromConfig(dutConfig)
	} else if dutInfo, err = cellularcommon.NewDUTInfoFromStringArgs(ctx, s.Var, "autotest_host_info_labels"); err != nil {
		s.Fatal("Failed to get SIM info labels: ", err)
	}
	helper.SetDUTInfo(dutInfo)
	dutInfo.LogInfo(ctx)

	starfish, _, _, err := starfish.NewStarfish(ctx)

	if starfish == nil {
		s.Fatal("Starfish setup failed")
	}

	slots, _ := starfish.AvailableSimSlots(ctx)
	if len(slots) < 2 {
		s.Fatal("Atleast 2 SIMs needed in starfish for SIM swap")
	}

	starfishSlotLabel := helper.GetLabelStarfishSlotMapping(ctx)
	slotCarrierMap := starfish.ParseStarfishSlotMapping(starfishSlotLabel)
	s.Log("Starfish slot Map", slotCarrierMap)
	if len(slotCarrierMap) < 2 {
		s.Fatal("Atleast 2 SIMs needed in starfish label for SIM swap")
	}

	err = starfish.SimInsert(ctx, slots[0])
	if err != nil {
		s.Fatal("Failed to insert SIM in starfish index: ", slots[0], err)
	}
	carrier := slotCarrierMap[slots[0]]

	s.Log("Starfish Index: ", slots[0], " carrier: ", carrier)

	// Ensure that a Cellular Service was created.
	if _, err := helper.FindService(ctx); err != nil {
		s.Fatal("Unable to find Cellular Service: ", err)
	}

	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	if err := modem.Enable(ctx); err != nil {
		s.Fatal("Modem enable failed with: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	gaiaCreds, err := credconfig.PickRandomCreds(s.RequiredVar("cellular.gaiaAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse cellular user creds: ", err)
	}

	s.Log("Create and upload CSV file to lock the device")
	// Create and upload appropriate lock file for Verizon
	err = helper.CreateAndUploadCarrierLockCsv(ctx, gaiaCreds, cellular.SimLockVzwProfileID)
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

	if carrier != "verizon" {
		s.Log("Wait for sim lock type to change to network-pin")
		if err = helper.WaitForCarrierLock(ctx, true); err != nil {
			s.Fatal("Card not reported as network-pin locked: ", carrier)
		}
	} else {
		s.Log("Wait for sim lock type to change to none")
		if err = helper.WaitForCarrierLock(ctx, false); err != nil {
			s.Fatal("Card not reported as unlocked: ", carrier)
		}
	}

	s.Log("Performing SIM swap")
	err = starfish.SimInsert(ctx, slots[1])
	if err != nil {
		s.Fatal("Failed to perform  SIM swap in starfish: ", slots[1], err)
	}
	carrier = slotCarrierMap[slots[1]]

	s.Log("Starfish Index after SIM swap: ", slots[1], " carrier: ", carrier)

	if carrier != "verizon" {
		s.Log("Wait for sim lock type to change to network-pin")
		if err = helper.WaitForCarrierLock(ctx, true); err != nil {
			s.Fatal("Card not reported as network-pin locked: ", carrier)
		}
	} else {
		s.Log("Wait for sim lock type to change to none")
		if err = helper.WaitForCarrierLock(ctx, false); err != nil {
			s.Fatal("Card not reported as unlocked: ", carrier)
		}
	}
}
