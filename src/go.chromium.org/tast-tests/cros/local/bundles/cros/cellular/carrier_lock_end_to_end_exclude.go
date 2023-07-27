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
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CarrierLockEndToEndExclude,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that carriers excluded by carrier lock config does not connect",
		Contacts:     []string{"ujjwalpande@google.com", "chromeos-cellular-team@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:      "cellularSIMLockCleared",
		SoftwareDeps: []string{"chrome"},
		Timeout:      20 * time.Minute,
		VarDeps:      []string{"cellular.gaiaAccountPool"},
	})
}

// CarrierLockEndToEndExclude validates that excluded MCC/MNCs from carrier
// lock config could not connect.
func CarrierLockEndToEndExclude(ctx context.Context, s *testing.State) {
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
	// Create and upload appropriate lock file to exclude Verizon MCC/MNCs
	err = helper.CreateAndUploadCarrierLockCsv(ctx, gaiaCreds, cellular.SimLockExcludeVzwProfileID)
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
		faillog.DumpUITree(ctx, s.OutDir(), uiHelper.Tconn)
		s.Fatal("Failed to create cellular.NewUiHelper: ", err)
	}
	uiHelper.LaunchChromeWithCarrierLock(ctx, gaiaCreds.User, gaiaCreds.Pass)

	s.Log("Wait for service to come up and get fresh config")

	// GoBigSleepLint: Wait for carrier lock service to get fresh config
	testing.Sleep(ctx, 60*time.Second)

	s.Log("Test Connect after locking")
	_, err = helper.Connect(ctx)

	if err == nil && carrier == "NETWORK_VERIZON" {
		s.Fatal("Connect succeeded expectedly after applying carrier lock. Current carrier: ", carrier)
	}

	if err != nil && carrier != "NETWORK_VERIZON" {
		s.Fatal("Connect failed unexpectedly after applying carrier lock. Current carrier: ", carrier)
	}

	s.Log("upload the unlock config")
	// Create and upload appropriate unlock file
	err = helper.CreateAndUploadCarrierLockCsv(ctx, gaiaCreds, cellular.SimLockUnlockProfileID)
	if err != nil {
		s.Fatal("Failed to create and upload CSV file: ", err)
	}

	s.Log("Wait for FCM notification")

	// GoBigSleepLint: Wait for FCM notification and carrier lock service to fetch new
	// unlock configuration.
	// There is no way to know if FCM notification was received and processed by
	// carrier lock manager other than to wait for predefined time.
	testing.Sleep(ctx, 3*time.Minute)

	s.Log("Test Connect after unlocking")
	// Ensure that cellular can connect after carrier lock is removed
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service after carrier unlock: ", err)
	}
}
