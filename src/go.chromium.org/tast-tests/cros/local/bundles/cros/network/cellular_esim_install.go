// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularESimInstall,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the add eSIM profile via activation code flow in the success and failure cases",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_test_esim"},
		Fixture:      "cellularTestESIM",
		Timeout:      9 * time.Minute,
	})
}

func CellularESimInstall(ctx context.Context, s *testing.State) {
	euicc, slot, err := hermes.GetEUICC(ctx, true)
	if err != nil {
		s.Fatal("Failed to get test euicc: ", err)
	}

	// Remove any existing profiles on test euicc
	if err := euicc.ResetMemory(ctx); err != nil {
		s.Fatal("Failed to reset test euicc: ", err)
	}
	defer euicc.ResetMemory(ctx)
	s.Log("Reset test euicc completed")

	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodUseTestCerts, true).Err; err != nil {
		s.Fatal("Failed to set use test cert on eUICC: ", err)
	}
	s.Log("Set to use test cert on euicc completed")

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	chromeOpts := []chrome.Option{
		chrome.EnableFeatures("UseStorkSmdsServerAddress"),
	}
	if slot == 1 {
		s.Log("Append CellularUseSecondEuicc feature flag")
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("CellularUseSecondEuicc"))
	}

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	activationCode, cleanupFunc, err := stork.FetchStorkProfile(ctx)
	if err != nil {
		s.Fatal("Failed to fetch Stork profile: ", err)
	}
	defer cleanupFunc(ctx)

	s.Log("Fetched Stork profile with activation code: ", activationCode)

	if err := ossettings.AddESimWithActivationCode(ctx, tconn, string(activationCode)); err != nil {
		s.Fatal("Failed to add esim profile with correct activation code: ", err)
	}

	// May see error at the end of user journey because the stork profile is not able to connect,
	// and the entire installation process will attempt to connect to the newly installed profile
	// after Hermes installation.
	if err := uiauto.Combine("Exit add cellular eSIM flow after using correct activation code",
		mdp.WithTimeout(3*time.Minute).LeftClick(ossettings.DoneButton.Focusable()),
	)(ctx); err != nil {
		s.Fatal("Correct activation code user journey fails: ", err)
	}

	// Verify the newly installed profile appears in the network list
	if err := ossettings.VerifyCelluarNetworkExistInList(ctx, tconn, "CarrierConfirmationCode"); err != nil {
		s.Fatal("Failed to verify newly installed stork profile: ", err)
	}
}
