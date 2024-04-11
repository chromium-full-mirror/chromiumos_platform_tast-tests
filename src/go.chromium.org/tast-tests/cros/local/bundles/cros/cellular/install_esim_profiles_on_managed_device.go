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
	"go.chromium.org/tast-tests/cros/local/networkui/netconfig"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Buffer time after resetting the EUICC before continuing.
const resetEUICCMemoryTimeout = 30 * time.Second

type installESIMProfilesOnManagedDeviceTestConfig struct {
	useSMDS bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         InstallESIMProfilesOnManagedDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that managed eSIM profile can be installed from device policy via the esim_manager Mojo API",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@google.com",
			"khegde@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_sim_test_esim", "cellular_e2e"},
		Fixture:      "cellularWithFakeDMSEnrolledAndTestSIM",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
		Timeout: 15 * time.Minute,
		Params: []testing.Param{
			{
				Name: "smdp",
				Val: &installESIMProfilesOnManagedDeviceTestConfig{
					useSMDS: false,
				},
			},
			{
				Name: "smds",
				Val: &installESIMProfilesOnManagedDeviceTestConfig{
					useSMDS: true,
				},
			},
		},
	})
}

// InstallESIMProfilesOnManagedDevice ensures that eSIM operations work with a Stork server when accessed via Mojo.
func InstallESIMProfilesOnManagedDevice(ctx context.Context, s *testing.State) {
	euicc, slot, err := hermes.GetEUICC(ctx, true)
	if err != nil {
		s.Fatal("Failed to get eUICC via hermes: ", err)
	}

	// Remove any existing profiles on test eUICC.
	if err := resetEUICCMemory(ctx, euicc); err != nil {
		testing.ContextLog(ctx, "Failed to reset eUICC memory")
	}
	s.Log("Reset eUICC completed")

	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodUseTestCerts, true).Err; err != nil {
		s.Fatal("Failed to set use_test_cert on eUICC: ", err)
	}
	s.Log("Set to use test cert on eUICC completed")

	eid, err := euicc.Eid(ctx)
	if err != nil {
		s.Fatal("Failed to get EID of eUICC: ", err)
	}

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr, err := startChromeWithFakeDMS(ctx, fdms, slot, true)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctxForCleanup)

	netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(ctxForCleanup)

	ctxForCleanupStork := ctx
	ctx, cancel = ctxutil.Shorten(ctx, stork.CleanupProfileTime)
	defer cancel()

	activationCodes, cleanupFunc, err := stork.FetchStorkProfilesForEid(ctx, eid, 1)
	if cleanupFunc != nil {
		defer cleanupFunc(ctxForCleanupStork)
	}
	if err != nil {
		s.Fatal("Failed to fetch the Stork profile: ", err)
	}

	if len(activationCodes) != 1 {
		s.Fatalf("Unexpected number of Stork profiles fetched, got: %v, want: 1", len(activationCodes))
	}

	activationCode := activationCodes[0]
	s.Log("Fetched Stork profile with activation code: ", activationCode)

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

	if err := netConn.WaitForCellularDeviceUninhibited(ctx); err != nil {
		s.Fatal("Failed to get uninhibited cellular device: ", err)
	}

	// Save the profile ICCID for verification.
	profileICCID, err := getProfileICCID(ctx, mojoEuicc)
	if err != nil {
		s.Fatal("Failed to get profile ICCID: ", err)
	}

	if err := netConn.WaitForCellularDeviceUninhibited(ctx); err != nil {
		s.Fatal("Failed to get uninhibited cellular device: ", err)
	}

	ctxForCleanupResetEuicc := ctx
	ctx, cancel = ctxutil.Shorten(ctx, resetEUICCMemoryTimeout)
	defer cancel()

	// Defer an attempt to reset the eUICC memory before installing the eSIM profile in case the installation spuriously failed but still installed a profile.
	defer func() {
		if err := resetEUICCMemory(ctxForCleanupResetEuicc, euicc); err != nil {
			testing.ContextLog(ctxForCleanupResetEuicc, "Failed to reset eUICC memory")
		}
	}()

	if s.Param().(*installESIMProfilesOnManagedDeviceTestConfig).useSMDS {
		err = installESIMProfileViaPolicyWithSMDS(ctx, euicc, fdms, cr)
	} else {
		err = installESIMProfileViaPolicyWithSMDP(ctx, euicc, fdms, cr, string(activationCode))
	}
	if err != nil {
		s.Fatal("Failed to install eSIM profile: ", err)
	}
	s.Log("Applied device policy with managed cellular network configuration")

	// Installing an eSIM profile should inhibit the device.
	// Wait here until this happens to make sure that the policy was applied and the installation begins.
	if err := netConn.WaitForCellularDeviceInhibited(ctx); err != nil {
		s.Fatal("Failed to wait for cellular device to be inhibited before installation: ", err)
	}

	if err := netConn.WaitForCellularDeviceUninhibited(ctx); err != nil {
		s.Fatal("Failed to wait for cellular device to finish installing and not be inhibited: ", err)
	}
	if err := verifyTestESIMProfileWasInstalled(ctx, mojoEuicc, profileICCID); err != nil {
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

func installESIMProfileViaPolicyWithSMDP(ctx context.Context, euicc *hermes.EUICC, fdms *fakedms.FakeDMS, cr *chrome.Chrome, activationCode string) error {
	networkConfig := &policy.ONCCellular{
		SMDPAddress: string(activationCode),
	}
	return installESIMProfileViaPolicy(ctx, euicc, fdms, cr, networkConfig)
}

func installESIMProfileViaPolicyWithSMDS(ctx context.Context, euicc *hermes.EUICC, fdms *fakedms.FakeDMS, cr *chrome.Chrome) error {
	networkConfig := &policy.ONCCellular{
		SMDSAddress: stork.SMDSActivationCodePrefix,
	}
	return installESIMProfileViaPolicy(ctx, euicc, fdms, cr, networkConfig)
}

func installESIMProfileViaPolicy(ctx context.Context, euicc *hermes.EUICC, fdms *fakedms.FakeDMS, cr *chrome.Chrome, networkConfig *policy.ONCCellular) error {
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
					Cellular: networkConfig,
				},
			},
		},
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
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("SmdsSupport"))
	}
	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		return nil, err
	}

	return cr, nil
}

func verifyTestESIMProfileWasInstalled(ctx context.Context, e *mojo.Euicc, profileICCID string) error {
	availableProfiles, err := e.ProfileList(ctx)
	if err != nil {
		return errors.Wrap(err, "error requesting available profiles")
	}
	numProfiles := len(availableProfiles)
	if numProfiles == 0 {
		return errors.New("failed to get any profiles")
	}
	if numProfiles > 1 {
		testing.ContextLogf(ctx, "Expected 1 profile, found %d", numProfiles)
	}
	for _, profile := range availableProfiles {
		if profile.Iccid != profileICCID {
			continue
		}
		properties, err := profile.Properties(ctx)
		if err != nil {
			return errors.New("failed to retrieve properties for profile with matching ICCID")
		}
		if properties.State == mojo.ProfileStatePending || properties.State == mojo.ProfileStateInstalling {
			return errors.Errorf("unexpected profile state, got: %d, want: %d or %d", properties.State, mojo.ProfileStateInactive, mojo.ProfileStateActive)
		}
		return nil
	}
	return errors.Errorf("failed to find installed profile with ICCID of %s", profileICCID)
}

func resetEUICCMemory(ctx context.Context, euicc *hermes.EUICC) error {
	if err := euicc.ResetMemory(ctx); err != nil {
		return err
	}
	testing.ContextLog(ctx, "eUICC memory is being reset")

	// Poll until there are no installed profiles on the eUICC.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		profiles, err := euicc.InstalledProfiles(ctx, true /*shouldNotSwitchSlot*/)
		if err != nil {
			return err
		}
		if len(profiles) != 0 {
			return errors.New("failed to remove installed eSIM profiles")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  resetEUICCMemoryTimeout,
		Interval: 5 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to remove installed eSIM profiles")
	}
	return nil
}
