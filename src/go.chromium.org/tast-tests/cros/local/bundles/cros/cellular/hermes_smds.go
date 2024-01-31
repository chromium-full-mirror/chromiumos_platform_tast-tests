// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HermesSMDS,
		Desc:         "Perform SMDS eSIM operations on test eSIM",
		Contacts:     []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_test_esim"},
		Fixture:      "cellularTestESIM",
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			// Calls Hermes.Euicc.RefreshSmdxProfiles and ensures non-empty activation codes.
			Name: "refresh",
			Val:  "",
		}, {
			// Calls Hermes.Euicc.RefreshSmdxProfiles then installs and uninstalls the profiles.
			Name: "install",
			Val:  "install",
		}},
	})
}

// HermesSMDS ensures that Hermes SMDS eSIM operations work with a stork server.
func HermesSMDS(ctx context.Context, s *testing.State) {
	testVal, ok := s.Param().(string)
	if !ok {
		s.Fatal("Unable to read test mode")
	}

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

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, stork.CleanupProfileTime)
	defer cancel()

	const numProfiles = 2
	_, cleanupFunc, err := stork.FetchStorkProfilesForEid(ctx, eid, numProfiles)
	if cleanupFunc != nil {
		defer cleanupFunc(ctxForCleanup)
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
		s.Logf("Pending profile %s", profile.String())
		if testVal == "install" {
			if err := hermes.InstallProfile(ctx, euicc, profile); err != nil {
				s.Fatalf("Failed to install profile %q: %v", profile.String(), err)
			}
			if err := hermes.UninstallProfile(ctx, euicc, profile); err != nil {
				s.Fatalf("Failed to uninstall profile %q: %v", profile.String(), err)
			}
		} else {
			code, err := profile.ActivationCode(ctx)
			if err != nil {
				s.Fatal("Failed to get activation code: ", err)
			}
			if code == "" {
				s.Fatal("Activation code is empty")
			}
		}
	}
}
