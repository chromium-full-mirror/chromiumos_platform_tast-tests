// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CarrierLockEndToEndESIMInstall,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that carrier lock restrictions are enforced by modem",
		Contacts:     []string{"chromeos-cellular-team@google.com", "ujjwalpande@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_test_esim"},
		Fixture:      "cellularTestESIM",
		SoftwareDeps: []string{"chrome"},
		Timeout:      20 * time.Minute,
		VarDeps:      []string{"cellular.gaiaAccountPool"},
		Params: []testing.Param{
			{
				Name:              "",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("yavilla")),
			},
		},
	})
}

// CarrierLockEndToEndESIMInstall validates esim install/uninstall
// is allowed on carrier locked device.
func CarrierLockEndToEndESIMInstall(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	helper.PrintSIMInfo(ctx)

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

	s.Log("Wait for service to come up and get fresh config")

	// GoBigSleepLint: Wait for carrier lock service to get fresh config
	testing.Sleep(ctx, 60*time.Second)

	s.Log("Install test esim profile")

	// Get a test euicc.
	euicc, _, err := hermes.GetEUICC(ctx, true)
	if err != nil {
		s.Fatal("Unable to get Hermes euicc: ", err)
	}

	if err := euicc.Call(ctx, hermesconst.EuiccMethodUseTestCerts, true).Err; err != nil {
		s.Fatal("Failed to use test certs: ", err)
	}
	s.Log("Using test certs")

	eid, err := euicc.Eid(ctx)
	if err != nil {
		s.Fatal("Unable to get Hermes eid: ", err)
	}
	s.Log("EID of the euicc: ", eid)

	const numProfiles = 2
	_, cleanupFunc, err := stork.FetchStorkProfilesForEid(ctx, eid, numProfiles)
	if cleanupFunc != nil {
		defer cleanupFunc(cleanupCtx)
	}
	if err != nil {
		s.Fatal("Failed to fetch Stork profile: ", err)
	}

	pendingProfiles, err := euicc.RefreshSmdxProfiles(ctx, hermesconst.RootSmdsAddress, false)
	if err != nil {
		s.Fatal("Failed to get pending profiles: ", err)
	}
	if len(pendingProfiles) < numProfiles {
		s.Fatalf("Got %d profiles, want %d profiles", len(pendingProfiles), numProfiles)
	}

	for _, profile := range pendingProfiles {
		s.Logf("Installing and uninstalling profile %s", profile.String())
		if err := hermes.InstallProfile(ctx, euicc, profile); err != nil {
			s.Fatalf("Failed to install profile %q: %v", profile.String(), err)
		}
		if err := hermes.UninstallProfile(ctx, euicc, profile); err != nil {
			s.Fatalf("Failed to uninstall profile %q: %v", profile.String(), err)
		}
	}
}
