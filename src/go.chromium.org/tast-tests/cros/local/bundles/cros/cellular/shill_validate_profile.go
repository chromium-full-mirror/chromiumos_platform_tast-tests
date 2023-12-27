// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type shillValidateProfileTestParam struct {
	TestNewAPNUIRevamp bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillValidateProfile,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that the cellular profile is still valid after a shill/OS upgrade",
		Contacts:       []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Data:           []string{"callbox_attach_ipv4_incorrect_apn.pbf", "test_profile.txt"},
		Fixture:        "cellular",
		Timeout:        3 * time.Minute,
		Params: []testing.Param{{
			Name: "old_ui",
			Val:  shillValidateProfileTestParam{false},
		}, {
			Name: "revamp_apn_ui",
			Val:  shillValidateProfileTestParam{true},
		}},
	})
}

// ShillValidateProfile Validates profile apn changes before and after the shill reset.
func ShillValidateProfile(ctx context.Context, s *testing.State) {
	const (
		incorrectAPNProto  = "callbox_attach_ipv4_incorrect_apn.pbf"
		testDefaultProfile = "test_profile.txt"
		tempFilePath       = "/var/cache/shill/temp_profile.txt"
		apn                = "callbox-ipv4"
	)
	params := s.Param().(shillValidateProfileTestParam)
	testNewAPNUIRevamp := params.TestNewAPNUIRevamp
	// Test steps:
	// - Verify that shill connects with an empty profile and default modb.
	// - Load incorrectAPNProto and verify shill connects with the fallback APN.
	// - Load a test profile containing a valid pre-upgrade configuration.
	// The test will fail if shill fails to load and use the pre-upgrade configuration.

	helper := s.FixtValue().(*cellular.FixtData).Helper
	// Fail early on NL668, otherwise the modem will keep returning WriteFailure on SetInitialEPSBearerSettings.
	nl668Err := cellular.TagKnownBugOnModem(ctx, nil, "b/217563991", cellular.ModemFwFilterNL668A01)
	if nl668Err != nil {
		s.Fatalf("Fail early to avoid wasting DUT time: %s", nl668Err)
	}

	// Check cellular connection for default profile. Shill will connect with any APN, so don't check the APN.
	if connected, err := checkCellularConnection(ctx, helper, true); err != nil || !connected {
		s.Fatal("Failed to connect with the default settings: ", err)
	}

	cleanup, err := cellular.SetServiceProvidersExclusiveOverride(ctx, s.DataPath(incorrectAPNProto))
	if err != nil {
		s.Fatal("Failed to set service providers override: ", err)
	}
	defer cleanup()

	s.Log("Incorrect apn textproto loaded. Reset shill")
	errs := helper.ResetShill(ctx)
	if errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}
	// Because the modb only contains invalid APNs, shill will connect with the fallback APN,
	// or it will fail to connect on modems/FWs that don't correctly return the InvalidApn error.
	connected, err := checkCellularConnection(ctx, helper, false)
	if err != nil {
		s.Fatal("Failed to check for cellular connection: ", err)
	} else if connected {
		if err := checkAPN(ctx, helper, ""); err != nil {
			s.Fatal("Failed checking APN: ", err)
		}
	}
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}
	// Create profile by reading iccid, imsi from sim card and write to file.
	iccid, err := modem.GetSimIdentifier(ctx)
	if err != nil {
		s.Fatal("Failed to read SIM Identifier from modemmanager: ", err)
	}
	imsi, err := modem.GetIMSI(ctx)
	if err != nil {
		s.Fatal("Failed to read SIM IMSI: ", err)
	}
	path := s.DataPath(testDefaultProfile)
	testProfile, err := ioutil.ReadFile(path)
	if err != nil {
		s.Fatal("Could not read test profile from given profile file: ", err)
	}

	newProfile := strings.Replace(string(testProfile), "#ICCID_NUMBER#", iccid, -1)
	newProfile = strings.Replace(string(newProfile), "#IMSI_NUMBER#", imsi, -1)
	// When using the revamp, keep both values, since most users will still have the old `Cellular.APN` values, as those are not deleted.
	apnInfo := "Cellular.APN.version=2\nCellular.APN.apn_types=DEFAULT,IA\nCellular.APN.apn=" + apn
	if testNewAPNUIRevamp {
		apnInfo = apnInfo + "\n" + `Cellular.CustomAPNList=[{"apn":"` + apn + `","apn_source":"ui","apn_types":"DEFAULT,IA","id":"99","version":"2"}]`
	}
	newProfile = strings.Replace(string(newProfile), "#APN_INFO#", apnInfo, -1)

	s.Log("After update: ", newProfile)
	if err := os.WriteFile(tempFilePath, []byte(newProfile), 0600); err != nil {
		s.Fatal("Failed to write updated test profile to path: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		err := os.Remove(tempFilePath)
		if err != nil && !os.IsNotExist(err) {
			s.Log("Failed to remove temp profile: ", err)
		}
	}(cleanupCtx)

	s.Log("Test profile loaded. Reset shill")
	errs = helper.ResetShillAndSetProfile(ctx, tempFilePath)
	if errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	// Shill should connect with the APN in the pre-upgrade test profile.
	if connected, err := checkCellularConnection(ctx, helper, true); err != nil || !connected {
		s.Fatal("Failed to connect with the pre-upgrade profile: ", err)
	}
	if err := checkAPN(ctx, helper, apn); err != nil {
		s.Fatal("Failed checking APN: ", err)
	}
	s.Log("Successfully connected with side loaded default profile - Step3 Done")

}

// checkCellularConnection tries to connect and returns the connected status. If |connectionMustSucceed| is false,
// a failure to connect will not result in an error.
func checkCellularConnection(ctx context.Context, helper *cellular.Helper, connectionMustSucceed bool) (bool, error) {
	// Check cellular connection, should able to connect with default apn(ipv4 one).
	if _, err := helper.Enable(ctx); err != nil {
		return false, errors.Wrap(err, "failed to enable cellular")
	}

	// Verify that a connectable Cellular service exists and ensure it is connected.
	service, err := helper.FindServiceForDevice(ctx)
	if err != nil {
		return false, errors.Wrap(err, "unable to find cellular service for device")
	}
	isConnected, err := service.IsConnected(ctx)
	if err != nil {
		return false, errors.Wrap(err, "unable to get isConnected for service")
	}
	testing.ContextLog(ctx, "Connecting")
	if !isConnected {
		if err := helper.ConnectToServiceWithTimeout(ctx, service, time.Second*10); err != nil && connectionMustSucceed {
			return false, errors.Wrap(err, "unable to connect to service")
		}
		isConnected, _ = service.IsConnected(ctx)
	}
	testing.ContextLog(ctx, "No error and isConnected val : ", isConnected)
	return isConnected, nil
}

func checkAPN(ctx context.Context, helper *cellular.Helper, expectedAPN string) error {
	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to get service properties")
	}
	testing.ContextLog(ctx, "serviceAPN: ", serviceLastGoodAPN)
	apn := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	if expectedAPN != apn {
		return errors.Errorf("last good APN doesn't match: got %q, want %q", apn, expectedAPN)
	}
	return nil
}
