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

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillValidateProfile,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that change in profile property able to connect after shill reset, mimics OS update",
		Contacts:       []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Data:           []string{"callbox_attach_ipv4_incorrect_apn.pbf", "test_profile.txt"},
		Fixture:        "cellular",
		Timeout:        5 * time.Minute,
	})
}

// ShillValidateProfile Validates profile apn changes before and after the shill reset.
func ShillValidateProfile(ctx context.Context, s *testing.State) {
	const (
		incorrectAPNProto  = "callbox_attach_ipv4_incorrect_apn.pbf"
		testDefaultProfile = "test_profile.txt"
		tempFilePath       = "/var/cache/shill/temp_profile.txt"
	)
	// Create incorrect proto which fails and convert to pbf and load
	// default profile exist at /var/cache/shill/default.profile
	// Load default profile and able to connect successfully to callbox (apn)
	// Load incorrect textproto apn
	// Call ResetShill
	// Check for service failure to connect to callbox (with incorrect apn)
	// Call ResetShill With Profile a placeholder profile path to sideload test_profile created from default profile and added APN
	// Should able to connect default.profile

	helper := s.FixtValue().(*cellular.FixtData).Helper
	// Fail early on NL668, otherwise the modem will keep returning WriteFailure on SetInitialEPSBearerSettings.
	nl668Err := cellular.TagKnownBugOnModemType(ctx, nil, "b/217563991", []cellularconst.ModemType{cellularconst.ModemTypeNL668})
	if nl668Err != nil {
		s.Fatalf("Fail early to avoid wasting DUT time: %s", nl668Err)
	}

	// Check cellular connection for default profile.
	if connected, err := checkCellularConnection(ctx, helper, true); err != nil || !connected {
		s.Fatal("Supposed to connect with the given good apn proto configuration: ", err)
	}
	s.Log("Connected with default profile - Step1 Done")

	printShillInfo(ctx, helper)

	cleanup, err := cellular.SetServiceProvidersExclusiveOverride(ctx, s.DataPath(incorrectAPNProto))
	if err != nil {
		s.Fatal("Failed to set service providers override: ", err)
	}
	defer cleanup()

	s.Log("Reset Shill after loading incorrect apn textproto")
	errs := helper.ResetShill(ctx)
	if errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	// Check profile properties after modification.
	s.Log("Loaded incorrect apn textproto and after resetshill")
	printShillInfo(ctx, helper)

	// Check cellular connection, should fail to connect with incorrect apn(ipv4 incorrect one), and do not try to connect to default profile(false).
	if connected, err := checkCellularConnection(ctx, helper, false); err != nil || connected {
		s.Fatal("Supposed to fail in connecting as incorrect apn loaded: ", err)
	}
	s.Log("Should not connect as its incorrect profile - Step2 Done")

	printShillInfo(ctx, helper)
	s.Log("Reset shill with test default profile side load")

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

	newProfile := strings.Replace(string(testProfile), "iccidnumber", iccid, -1)
	newProfile = strings.Replace(string(newProfile), "imsinumber", imsi, -1)

	s.Log("After update: ", newProfile)
	if err := os.WriteFile(tempFilePath, []byte(newProfile), 0600); err != nil {
		s.Fatal("Could not write updated test profile to path: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 6*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		err := os.Remove(tempFilePath)
		if err != nil && !os.IsNotExist(err) {
			s.Log("Failed to remove temp profile: ", err)
		}
	}(cleanupCtx)

	// Reset shill and update profile properties in default profile.
	errs = helper.ResetShillAndSetProfile(ctx, tempFilePath)
	if errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	// Check cellular connection, should connect with default profile after shill reset.
	if connected, err := checkCellularConnection(ctx, helper, true); err != nil || !connected {
		s.Fatal("Supposed to connect with the given good apn proto configuration: ", err)
	}
	s.Log("Successfully connected with side loaded default profile - Step3 Done")
	printShillInfo(ctx, helper)
}

// checkCellularConnection checks for cellular connection and tries to connect if connect is 'True'.
func checkCellularConnection(ctx context.Context, helper *cellular.Helper, connect bool) (bool, error) {
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
	if !isConnected && connect {
		if _, err := helper.ConnectToDefault(ctx); err != nil {
			return false, errors.Wrap(err, "unable to connect to service")
		}
		isConnected, _ = service.IsConnected(ctx)
	}
	testing.ContextLog(ctx, "No error and isConnected val : ", isConnected)
	return isConnected, nil
}

// printShillInfo prints shill apns used to connect.
func printShillInfo(ctx context.Context, helper *cellular.Helper) error {
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		return errors.Wrap(err, "could not find mm dbus object with a valid sim")
	}
	modemAttachApn, err := modem.GetInitialEpsBearerSettings(ctx, modem)
	if err != nil {
		return errors.Wrap(err, "error getting Attach APN properties")
	}
	testing.ContextLog(ctx, "Modem attach APN     : ", modemAttachApn)
	bearer, err := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	if err != nil {
		return errors.Wrap(err, "error getting connected bearer properties")
	}
	apnName, err := bearer.GetAPN()
	if err != nil {
		return errors.Wrap(err, "error getting APN name")
	}
	testing.ContextLog(ctx, "Connected bearer APN : ", apnName)

	serviceLastAttachAPN, err := helper.GetCellularLastAttachAPN(ctx)
	if err != nil {
		return errors.Wrap(err, "error getting Service properties")
	}
	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	if err != nil {
		return errors.Wrap(err, "error getting Service properties")
	}
	testing.ContextLog(ctx, "Service last attach APN: ", serviceLastAttachAPN)
	testing.ContextLog(ctx, "Service last good APN  : ", serviceLastGoodAPN)

	return nil
}
