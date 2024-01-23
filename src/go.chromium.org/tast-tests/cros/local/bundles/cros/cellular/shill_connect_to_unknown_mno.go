// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
)

type unknownMNOTestParam struct {
	ModbOverrideProto     string
	ExpectedLastAttachAPN string
	ExpectedLastGoodAPN   string
	// Configure an Attach APN before starting the test.
	SetInitialAttachAPNValueInModemManager map[string]interface{}
	// Connect to APN
	ApnToConnect map[string]string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillConnectToUnknownMno,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that the cellular device can connect to a network with no information in the MODB",
		Contacts:       []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Params: []testing.Param{{
			Name: "unknown_carrier",
			Val: unknownMNOTestParam{
				"callbox_unknown_carrier.pbf", "callbox-default-attach", "callbox-ipv4",
				map[string]interface{}{mmconst.BearerPropertyApn: "wrong_attach", mmconst.BearerPropertyIPType: mmconst.BearerIPFamilyIPv4, mmconst.BearerPropertyApnType: mmconst.BearerAPNTypeInitial},
				map[string]string{shillconst.DevicePropertyCellularAPNInfoApnName: "callbox-ipv4", shillconst.DevicePropertyCellularAPNInfoApnSource: "ui", shillconst.DevicePropertyCellularAPNInfoApnIPType: shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4, shillconst.DevicePropertyCellularAPNInfoApnTypes: shillconst.DevicePropertyCellularAPNInfoApnTypeDefault},
			},
			ExtraData: []string{"callbox_unknown_carrier.pbf"},
		}},
		Fixture: "cellularResetShillProfileOnPostTest",
		Timeout: 1 * time.Minute,
	})
}

func ShillConnectToUnknownMno(ctx context.Context, s *testing.State) {
	params := s.Param().(unknownMNOTestParam)
	modbOverrideProto := params.ModbOverrideProto
	expectedLastGoodAPN := params.ExpectedLastGoodAPN
	expectedLastAttachAPN := params.ExpectedLastAttachAPN
	setInitialAttachAPNValueInModemManager := params.SetInitialAttachAPNValueInModemManager
	apnToConnect := params.ApnToConnect

	helper := s.FixtValue().(*cellular.FixtData).Helper
	isNL668, _ := cellular.IsModemType(ctx, cellularconst.ModemTypeNL668)
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find mm dbus object with a valid sim: ", err)
	}

	modem3gpp, err := modem.GetModem3gpp(ctx)
	if err != nil {
		s.Fatal("Could not get modem3gpp object: ", err)
	}
	if setInitialAttachAPNValueInModemManager != nil {
		if err := modemmanager.SetInitialEpsBearerSettings(ctx, modem3gpp, setInitialAttachAPNValueInModemManager); err != nil {
			s.Fatal("Failed to set initial EPS bearer settings: ", err)
		}
	}

	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Failed to disable cellular: ", err)
	}

	deferCleanUp, err := cellular.SetServiceProvidersExclusiveOverride(ctx, s.DataPath(modbOverrideProto))
	if err != nil {
		s.Fatal("Failed to set service providers override: ", err)
	}
	defer deferCleanUp()

	errs := helper.ResetShill(ctx)
	if errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Failed to enable cellular: ", err)
	}

	// Verify that a connectable Cellular service exists and ensure it is connected.
	if _, err := helper.FindServiceForDevice(ctx); err != nil {
		s.Fatal("Unable to find Cellular Service for Device: ", err)
	}

	if err := modem.WaitForState(ctx, mmconst.ModemStateRegistered, 20*time.Second); err != nil {
		s.Fatal("Modem is not registered")
	}

	testing.ContextLog(ctx, "Connecting")
	apns := []map[string]string{apnToConnect}
	if err = helper.SetCustomAPNList(ctx, apns); err != nil {
		s.Fatal("Unable to set the custom APN: ", err)
	}
	_, err = helper.Connect(ctx)
	if err != nil {
		s.Fatal("Unable to connect to service: ", err)
	}

	modemAttachApn, err := modem.GetInitialEpsBearerSettings(ctx, modem)
	if err != nil {
		s.Fatal("Error getting Attach APN properties: ", err)
	}
	bearer, err := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	if err != nil {
		s.Fatal("Error getting Default APN properties: ", err)
	}

	testing.ContextLog(ctx, "modemAttachApn:", modemAttachApn)
	testing.ContextLog(ctx, "connectApn", bearer)
	lastAttachMightBeNull := isNL668 // b/217563991#comment137
	if apnName := modemAttachApn["apn"]; apnName != expectedLastAttachAPN && !(lastAttachMightBeNull && (apnName == nil || apnName == "")) {
		s.Fatalf("Last Attach APN doesn't match: got %q, want %q", apnName, expectedLastAttachAPN)
	}

	if apnName, err := bearer.GetAPN(ctx); err != nil {
		s.Fatal("Error getting APN name: ", err)
	} else if apnName != expectedLastGoodAPN {
		s.Fatalf("Last good APN doesn't match: got %q, want %q", apnName, expectedLastGoodAPN)
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
