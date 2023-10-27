// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
)

type shillCellularCustomAPNTestParam struct {
	TestNewAPNUIRevamp bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillCustomApn,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that the device can connect to cellular when using custom APNs",
		Contacts:       []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Data:           []string{"test_no_apns.pbf"},
		Params: []testing.Param{{
			Name: "set_apn",
			Val:  shillCellularCustomAPNTestParam{false},
		}, {
			Name: "set_user_apn_list",
			Val:  shillCellularCustomAPNTestParam{true},
		}},
		Fixture: "cellularResetShillProfileOnPostTest",
		Timeout: 6 * time.Minute,
	})
}

func ShillCustomApn(ctx context.Context, s *testing.State) {
	params := s.Param().(shillCellularCustomAPNTestParam)
	testNewAPNUIRevamp := params.TestNewAPNUIRevamp
	modbOverrideProto := "test_no_apns.pbf"
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim (precondition): ", err)
	}
	operatorID, err := modem.GetOperatorIdentifier(ctx)
	if err != nil {
		s.Fatal("Cannot get the OperatorIdentifier: ", err)
	}

	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper (precondition): ", err)
	}

	modem3gpp, err := modem.GetModem3gpp(ctx)
	if err != nil {
		s.Fatal("Could not get modem3gpp object: ", err)
	}

	// Shorten deadline to leave time for cleanup.
	clearAttachCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer func(ctx context.Context) {
		// b/253685780: This test sets a bad attach APN on purpose, so we need to fix the attach APN on
		// vilboz since clearing the attach APN is not possible in that variant.
		cellular.CheckIfVilbozVerizonAndFixAttachAPN(ctx)

		if err := modemmanager.SetInitialEpsBearerSettings(ctx, modem3gpp, map[string]interface{}{"apn": ""}); err != nil {
			testing.ContextLog(ctx, "Failed to clear the initial EPS bearer settings: ", err)
		}

		if testNewAPNUIRevamp {
			if err := helper.ClearCustomAPNList(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to clear cellular.CustomAPNList: ", err)
			}
		}
	}(clearAttachCtx)
	defer cancel()

	// Try to start the test with a bad attach APN to ensure that shill clears the attach APN if needed.
	// Some modems don't allow custom attach APNs, so a failure here should not fail the test.
	if err := modemmanager.SetInitialEpsBearerSettings(ctx, modem3gpp, map[string]interface{}{"apn": "wrong_attach", "ip-type": mmconst.BearerIPFamilyIPv4}); err != nil {
		testing.ContextLog(ctx, "Failed to set initial EPS bearer settings: ", err)
	}

	if _, err = helper.Disable(ctx); err != nil {
		s.Fatal("Failed to disable cellular: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 4*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		cellular.CheckIfL850VerizonAndFixDefaultAPN(ctx)
	}(cleanupCtx)

	deferCleanUp, err := cellular.SetServiceProvidersExclusiveOverride(ctx, s.DataPath(modbOverrideProto))
	if err != nil {
		s.Fatal("Failed to set service providers override: ", err)
	}
	defer deferCleanUp()

	if errs := helper.ResetShill(ctx); errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	if _, err = helper.Enable(ctx); err != nil {
		s.Fatal("Failed to enable cellular: ", err)
	}

	service, err := helper.FindServiceForDevice(ctx)
	if err != nil {
		s.Fatal("Unable to find Cellular Service for Device: ", err)
	}

	// Check that shill recognizes the MCCMNC in |modbOverrideProto|.
	if _, _, err = helper.GetHomeProviderFromShill(ctx); err != nil {
		s.Fatal("Failed to get HomeProvider from shill: ", err)
	}
	isTmobile := false
	if board, _ := cellular.GetCarrier(operatorID); board == cellular.CarrierTmobile {
		isTmobile = true
	}

	knownAPNs, err := cellular.GetKnownAPNsForOperator(operatorID)
	if err != nil {
		s.Fatal("Cannot find known APNs: ", err)
	}
	// For SetApn, if any connection succeeds, the APN will be set as the last good APN,
	// and it will be added to the end of the try list. In this case, any other APNs might
	// fail to connect, but they will fallback to the last good APN and succeed. Take this
	// into consideration when comparing the test APN to the last connected APN.
	lastMatchingGoodAPN := ""
	optionalAPNExist := false
	optionalAPNSucceeded := false
	counter := 0
	for _, knownAPN := range knownAPNs {

		isConnected, err := service.IsConnected(ctx)
		if err != nil {
			s.Fatal("Error getting IsConnected for service: ", err)
		}
		// Ensure the service is disconnected to avoid a race condition that happens after setting a new APN.
		// If the new APN causes a de-attach, shill might report Connected while MM is performing the reattach.
		if isConnected {
			if _, err := helper.Disconnect(ctx); err != nil {
				s.Fatal("Failed to disconnect: ", err)
			}
		}

		if testNewAPNUIRevamp {
			// Append all other APNs after the one we are testing if the current APN is an attach APN. It should work either way.
			apn := knownAPN.GetAPNForShill()
			wrongApn := make(map[string]string)
			for k, v := range apn {
				wrongApn[k] = v
			}
			// Add a different wrong-apn every time to avoid throttling algorithms in the modem.
			wrongApn[shillconst.DevicePropertyCellularAPNInfoApnName] = fmt.Sprintf("wrong-apn%d", counter)
			counter++
			// Start with a wrong APN to ensure the round robin works correctly.
			apns := []map[string]string{wrongApn}
			apns = append(apns, apn)
			if knownAPN.IsAttachAPN() {
				for _, knownAPN2 := range knownAPNs {
					apn2 := knownAPN2.GetAPNForShill()
					if !reflect.DeepEqual(apn2, apn) {
						apns = append(apns, apn2)
					}
				}
			}

			if err = helper.SetCustomAPNList(ctx, apns); err != nil {
				s.Fatal("Unable to set the custom APN: ", err)
			}
		} else {
			ipType, okIPType := knownAPN.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnIPType]
			auth, okAuth := knownAPN.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnAuthentication]
			if knownAPN.IsAttachAPN() {
				// Skip known ipv4v6 and ipv6 APNs, since Cellular.APN doesn't support the ip_type field.
				// Skip known PAP APNs, since Cellular.APN doesn't support the authentication field.
				if (okIPType && (ipType == shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4v6 || ipType == shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv6)) ||
					(okAuth && (auth == shillconst.DevicePropertyCellularAPNInfoApnAuthenticationPap)) {
					continue
				}
			}

			if okIPType {
				// Remove ip_type since the UI never sends that value.
				delete(knownAPN.APNInfo, shillconst.DevicePropertyCellularAPNInfoApnIPType)
			}
			if okAuth {
				// Remove authentication since the UI never sends that value.
				delete(knownAPN.APNInfo, shillconst.DevicePropertyCellularAPNInfoApnAuthentication)
			}

			if err = helper.SetAPN(ctx, knownAPN.GetAPNForShill()); err != nil {
				s.Fatal("Unable to set the custom APN: ", err)
			}

		}

		if knownAPN.Optional {
			optionalAPNExist = true
		}
		testing.ContextLog(ctx, "Testing APN: ", knownAPN.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnName])
		if err := helper.ConnectToServiceWithTimeout(ctx, service, 60*time.Second); err != nil {
			if knownAPN.Optional {
				continue
			}
			// Tmobile rarely provides an IPv4, so we cannot set the tmobile APN using SetApn because that API only allows IPv4.
			if isTmobile && !testNewAPNUIRevamp {
				err = cellular.TagKnownBug(ctx, err, "b/290425429")
			}
			s.Fatal("Unable to Connect to Service: ", err)
		}

		serviceLastAttachAPN, err := helper.GetCellularLastAttachAPN(ctx)
		if err != nil {
			s.Fatal("Error getting Service properties: ", err)
		}
		serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
		if err != nil {
			s.Fatal("Error getting Service properties: ", err)
		}
		testing.ContextLog(ctx, "serviceLastAttachAPN: ", serviceLastAttachAPN)
		testing.ContextLog(ctx, "serviceAPN: ", serviceLastGoodAPN)

		apn := serviceLastAttachAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
		expectedAPN := knownAPN.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnName]
		// Some US carriers allow any attach APN, so the test should not fail if the APN matches the wrong one inserted by the test.
		ignoreAttachMismatch := testNewAPNUIRevamp && strings.HasPrefix(apn, "wrong-apn")
		if knownAPN.IsAttachAPN() && apn != expectedAPN && !ignoreAttachMismatch {
			if knownAPN.Optional {
				continue
			}
			var err error = nil
			if apn == "" {
				err = cellular.TagKnownBugOnModemType(ctx, err, "b/245968064", []cellularconst.ModemType{cellularconst.ModemTypeL850})
			}
			s.Fatalf("Last Attach APN doesn't match: got %q, want %q. %s", apn, expectedAPN, cellular.ErrorToCleanString(err))
		}

		apn = serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
		// Tmobile allows any Default APN, so the test should not fail if the APN matches the wrong one inserted by the test.
		ignoreDefaultMismatch := testNewAPNUIRevamp && isTmobile && strings.HasPrefix(apn, "wrong-apn")
		if apn != expectedAPN && !ignoreDefaultMismatch {
			// We reach this point when shill connected to cellular, but with a different APN.
			// This is considered a failure to connect, unless the APN is optional.
			// This usually happens with SetApn if an early APN succeeds, but latter connections use a wrong APN.
			// SetApn still adds the LastGoodApn in the try list.
			if !testNewAPNUIRevamp && knownAPN.Optional && lastMatchingGoodAPN != "" && apn == lastMatchingGoodAPN {
				continue
			}
			s.Fatalf("Last good APN doesn't match: got %q, want %q", apn, expectedAPN)
		}
		lastMatchingGoodAPN = apn

		if knownAPN.Optional {
			optionalAPNSucceeded = true
		}

		ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
		if err != nil {
			s.Fatal("Failed to read network provisioned IP types: ", err)
		}
		s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)

		verifyHostIPConnectivity := func(ctx context.Context) error {
			if err := cellular.VerifyIPConnectivityUsingCurl(ctx, testexec.CommandContext, ipv4, ipv6, "/usr/bin"); err != nil {
				return errors.Wrap(err, "failed connectivity test")
			}
			return nil
		}

		if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPConnectivity); err != nil {
			s.Fatal("Failed to run test on cellular interface: ", err)
		}
	}

	if optionalAPNExist && !optionalAPNSucceeded {
		s.Fatal("None of the optional APNs connected")
	}
}
