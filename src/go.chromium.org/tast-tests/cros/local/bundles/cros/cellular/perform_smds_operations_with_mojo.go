// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/local/cellular/esim/mojo"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testConfig struct {
	shouldInstallProfiles bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           PerformSmdsOperationsWithMojo,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "An API test that performs SMDS eSIM operations on a test eSIM to validate the logic for the subset of functions in the esim_manager Mojo API (RequestAvailableProfiles, InstallProfileFromActivationCode)",
		Contacts: []string{
			"cros-network-health-team@google.com",
			"khegde@google.com",
			"chadduffin@google.com",
			"cros-connectivity@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_test_esim"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInWithMojoTestEuiccAndSmdsSupport",
		Timeout:      10 * time.Minute,
		Params: []testing.Param{{
			// Ensures non-empty activation codes.
			Name: "refresh",
			Val: &testConfig{
				shouldInstallProfiles: false,
			},
		}, {
			// Ensures profiles can be installed.
			Name: "install",
			Val: &testConfig{
				shouldInstallProfiles: true,
			},
		}},
	})
}

// PerformSmdsOperationsWithMojo ensures that eSIM operations work with a Stork server when accessed via Mojo.
func PerformSmdsOperationsWithMojo(ctx context.Context, s *testing.State) {
	shouldInstallProfiles := s.Param().(*testConfig).shouldInstallProfiles

	eSimMojo := s.FixtValue().(*mojo.FixtData)
	euicc := eSimMojo.Euicc

	cr := eSimMojo.Cr
	netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(ctx)

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, stork.CleanupProfileTime)
	defer cancel()

	const numProfiles = 2
	activationCodes, cleanupFunc, err := stork.FetchStorkProfilesForEid(ctx, euicc.Eid, numProfiles)
	if cleanupFunc != nil {
		defer cleanupFunc(ctxForCleanup)
	}
	if err != nil {
		s.Fatal("Failed to fetch Stork profile: ", err)
	}

	result, profiles, err := euicc.RequestAvailableProfiles(ctx)
	if err != nil {
		s.Fatal("Failed to request available profiles: ", err)
	}
	if result != mojo.ESimOperationSuccess {
		s.Fatalf("Failed to get eSIM operation success code, want: %v, got %v", mojo.ESimOperationSuccess, result)
	}

	if len(profiles) != numProfiles {
		s.Fatalf("Got %d profiles, want %d profiles", len(profiles), numProfiles)
	}

	for _, p := range profiles {
		if err := netConn.WaitForCellularDeviceUninhibited(ctx); err != nil {
			s.Fatal("Failed to get uninhibited cellular device: ", err)
		}

		if p.ActivationCode == "" {
			s.Fatal("Failed to get an activation code for profile: ", p.Iccid)
		}
		code := stork.ActivationCode(p.ActivationCode)
		if slices.Index(activationCodes, code) == -1 {
			s.Fatalf("Failed to find matching activation code %s for profile %v", p.ActivationCode, p.Iccid)
		}

		if !shouldInstallProfiles {
			continue
		}

		installResult, ep, err := euicc.InstallProfileFromActivationCode(ctx, p.ActivationCode, "" /*confirmationCode*/)
		if err != nil {
			s.Fatal("Failed to install eSIM profile via Mojo: ", err)
		}
		if installResult != mojo.ProfileInstallSuccess {
			s.Fatalf("eSIM install for profile %v, failed with error code: %v", p.Iccid, installResult)
		}
		if p.Iccid != ep.Iccid {
			s.Fatalf("Installed unexpected profile, got %v, want %v", ep.Iccid, p.Iccid)
		}

		result, err := ep.UninstallProfile(ctx)
		if result != mojo.ESimOperationSuccess {
			s.Fatal("Failed to uninstall profile ", ep.Iccid)
		}
	}
}
