// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/cellular/esim/mojo"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InstallESimProfilesOnManagedDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that managed eSIM profile can be installed from device policy via the esim_manager Mojo API",
		Contacts: []string{
			"cros-network-health-team@google.com",
			"khegde@google.com",
			"chadduffin@google.com",
			"cros-connectivity@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_test_esim", "cellular_e2e"},
		Fixture:      "cellularWithFakeDMSEnrolledAndTestSIM",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
		Timeout: 10 * time.Minute,
	})
}

// InstallESimProfilesOnManagedDevice ensures that eSIM operations work with a Stork server when accessed via Mojo.
func InstallESimProfilesOnManagedDevice(ctx context.Context, s *testing.State) {
	euicc, slot, err := hermes.GetEUICC(ctx, true)
	if err != nil {
		s.Fatal("Failed to get eUICC via hermes: ", err)
	}

	eid, err := euicc.Eid(ctx)
	if err != nil {
		s.Fatal("Failed to get EID of eUICC: ", err)
	}

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, stork.CleanupProfileTime)
	defer cancel()

	cr, err := startChromeWithFakeDMS(ctx, fdms, slot, true)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctxForCleanup)

	// Remove any existing profiles on test euicc
	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodResetMemory, 1).Err; err != nil {
		s.Fatal("Failed to reset test euicc: ", err)
	}
	s.Log("Reset test euicc completed")

	netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(ctxForCleanup)

	if err := netConn.WaitForCellularDeviceUninhibited(ctx); err != nil {
		s.Fatal("Failed to get uninhibited cellular device: ", err)
	}

	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodUseTestCerts, true).Err; err != nil {
		s.Fatal("Failed to set use test cert on eUICC: ", err)
	}
	s.Log("Set to use test cert on euicc completed")

	activationCodes, cleanupFunc, err := stork.FetchStorkProfilesForEid(ctx, eid, 1)
	if cleanupFunc != nil {
		defer cleanupFunc(ctxForCleanup)
	}
	if err != nil {
		s.Fatal("Failed to fetch the Stork profile: ", err)
	}

	if len(activationCodes) != 1 {
		s.Fatalf("Unexpected number of Stork profiles fetched, got: %v, want: 1", len(activationCodes))
	}

	s.Log("Fetched Stork profile with activation code: ", activationCodes[0])

	manager, err := mojo.Manager(ctx, cr, slot)
	if err != nil {
		s.Fatal("Failed to create Mojo interface to esim_manager")
	}

	euiccs, err := manager.AvailableEuicc(ctx)
	if err != nil {
		s.Fatal("Failed to get available eUICCs via Mojo: ", err)
	}

	if slot >= len(euiccs) {
		s.Fatalf("Failed to determine correct eUICC, slot index out of range, got=%v, want=%v(max)", slot, len(euiccs)-1)
	}
	mojoEuicc := &euiccs[slot]
	euiccProperties, err := mojoEuicc.Properties(ctx)
	if err != nil {
		s.Fatal("Error getting eUICCs properties via Mojo: ", err)
	}
	s.Log("Using eUICC: ", euiccProperties.Eid)

	// Save the profile ICCID for verification.
	profileICCID, err := getProfileICCID(ctx, mojoEuicc)
	if err != nil {
		s.Fatal("Failed to get profile ICCID: ", err)
	}

	err = installESimProfileViaPolicy(ctx, euicc, fdms, cr, string(activationCodes[0]))
	if err != nil {
		s.Fatal("Failed to install eSIM profile: ", err)
	}
	s.Log("Applied device policy with managed cellular network configuration")
	defer euicc.DBusObject.Call(ctxForCleanup, hermesconst.EuiccMethodResetMemory, 1)

	if err := netConn.WaitForCellularDeviceUninhibited(ctx); err != nil {
		s.Fatal("Failed to get uninhibited cellular device: ", err)
	}

	if err := verifyTestESimProfileWasInstalled(ctx, mojoEuicc, profileICCID); err != nil {
		s.Fatal("Failed to verify that eSIM profile was installed via device policy: ", err)
	}
}

func getProfileICCID(ctx context.Context, e *mojo.Euicc) (string, error) {
	result, availableProfiles, err := e.RequestAvailableProfiles(ctx)
	if err != nil {
		return "", errors.Wrap(err, "error requesting available profiles")
	}
	if result != mojo.ESimOperationSuccess {
		return "", errors.Errorf("unexpected esim operation result, got=%v, want=ESimOperationSuccess", result)
	}
	if len(availableProfiles) != 1 {
		return "", errors.Errorf("unexpected number of profile, got=%v, want=1", len(availableProfiles))
	}

	return availableProfiles[0].Iccid, nil
}

func installESimProfileViaPolicy(ctx context.Context, euicc *hermes.EUICC, fdms *fakedms.FakeDMS, cr *chrome.Chrome, activationCode string) error {
	cellularONC := &policy.ONCCellular{
		SMDPAddress: string(activationCode),
	}

	globalConfig := &policy.ONCGlobalNetworkConfiguration{
		AllowOnlyPolicyCellularNetworks: false,
	}

	deviceProfileServiceGUID := "Cellular-Device-Policy"
	deviceNetworkPolicy := &policy.DeviceOpenNetworkConfiguration{
		Val: &policy.ONC{
			GlobalNetworkConfiguration: globalConfig,
			NetworkConfigurations: []*policy.ONCNetworkConfiguration{
				{
					GUID:     deviceProfileServiceGUID,
					Name:     "CellularDevicePolicyName",
					Type:     "Cellular",
					Cellular: cellularONC,
				},
			},
		},
	}

	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodUseTestCerts, true).Err; err != nil {
		return errors.Wrap(err, "failed to set use test cert on test euicc")
	}

	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{deviceNetworkPolicy}); err != nil {
		return errors.Wrap(err, "failed to ServeAndRefresh ONC policy")
	}

	return nil
}

func startChromeWithFakeDMS(ctx context.Context, fdms *fakedms.FakeDMS, slot int, smdsSupportRequired bool) (*chrome.Chrome, error) {
	// Start a Chrome instance that will fetch policies from the FakeDMS.
	chromeOpts := []chrome.Option{
		chrome.EnableFeatures("UseStorkSmdsServerAddress"),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	}
	if slot == 1 {
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("CellularUseSecondEuicc"))
	}
	if smdsSupportRequired {
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("SmdsSupport", "SmdsSupportEuiccUpload", "SmdsDbusMigration"))
	}
	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		return nil, err
	}

	return cr, nil
}

func verifyTestESimProfileWasInstalled(ctx context.Context, e *mojo.Euicc, profileICCID string) error {
	availableProfiles, err := e.ProfileList(ctx)
	if err != nil {
		return errors.Wrap(err, "error requesting available profiles")
	}
	if len(availableProfiles) == 0 {
		return errors.New("failed to get any profiles")
	}

	if availableProfiles[0].Iccid != profileICCID {
		return errors.Errorf("profile ICCID mismatch, got=%s, want=%s", availableProfiles[0].Iccid, profileICCID)
	}

	return nil
}
