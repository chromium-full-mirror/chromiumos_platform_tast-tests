// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type unknownMNOTestParam struct {
	ModbOverrideProto     string
	ExpectedLastAttachAPN string
	ExpectedLastGoodAPN   string
	// Configure an Attach APN before starting the test.
	SetInitialAttachAPNValue map[string]interface{}
	// Connect to APN
	ApnToConnect map[string]interface{}
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillConnectToUnknownMno,
		Desc:         "Verifies that the cellular device can connect to a network with no information in the MODB",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Params: []testing.Param{{
			Name:      "unknown_carrier",
			Val:       unknownMNOTestParam{"callbox_unknown_carrier.pbf", "callbox-default-attach", "callbox-ipv4", map[string]interface{}{"apn": "wrong_attach", "ip-type": mmconst.BearerIPFamilyIPv4, "apn-type": mmconst.BearerAPNTypeInitial}, map[string]interface{}{"apn": "callbox-ipv4", "ip-type": mmconst.BearerIPFamilyIPv4, "apn-type": mmconst.BearerAPNTypeDefault}},
			ExtraData: []string{"callbox_unknown_carrier.pbf"},
		}},
		Fixture: "cellular",
		Timeout: 1 * time.Minute,
	})
}

func ShillConnectToUnknownMno(ctx context.Context, s *testing.State) {
	params := s.Param().(unknownMNOTestParam)
	modbOverrideProto := params.ModbOverrideProto
	expectedLastGoodAPN := params.ExpectedLastGoodAPN
	expectedLastAttachAPN := params.ExpectedLastAttachAPN
	setInitialAttachAPNValue := params.SetInitialAttachAPNValue
	apnToConnect := params.ApnToConnect

	helper := s.FixtValue().(*cellular.FixtData).Helper

	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find mm dbus object with a valid sim: ", err)
	}

	modem3gpp, err := modem.GetModem3gpp(ctx)
	if err != nil {
		s.Fatal("Could not get modem3gpp object: ", err)
	}
	if setInitialAttachAPNValue != nil {
		if err := modemmanager.SetInitialEpsBearerSettings(ctx, modem3gpp, setInitialAttachAPNValue); err != nil {
			s.Fatal("Failed to set initial EPS bearer settings: ", err)
		}
	}

	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Failed to disable cellular: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 6*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		// Restart shill after deleting |modbOverrideProto|.
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

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

	simpleModem, err := modem.GetSimpleModem(ctx)
	if err != nil {
		s.Fatal("Could not get simplemodem object: ", err)
	}

	testing.ContextLog(ctx, "Connecting")
	if _, err := modemmanager.Connect(ctx, simpleModem, apnToConnect); err != nil {
		s.Fatal("Modem connect failed with error: ", err)
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

	if apnName := modemAttachApn["apn"]; apnName != expectedLastAttachAPN {
		s.Fatalf("Last Attach APN doesn't match: got %q, want %q", apnName, expectedLastAttachAPN)
	}

	if apnName, err := bearer.GetAPN(); err != nil {
		s.Fatal("Error getting APN name: ", err)
	} else if apnName != expectedLastGoodAPN {
		s.Fatalf("Last good APN doesn't match: got %q, want %q", apnName, expectedLastGoodAPN)
	}
}
