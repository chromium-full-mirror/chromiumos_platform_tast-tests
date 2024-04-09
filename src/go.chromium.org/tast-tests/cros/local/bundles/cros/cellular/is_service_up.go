// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IsServiceUp,
		Desc:         "Verifies that Cellular Device and Service properties match ModemManager SIM properties",
		Contacts:     []string{"chromeos-cellular-team@google.com", "ejcaruso@google.com", "pholla@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_cq", "cellular_ota_avl"},
		Fixture:      "cellular",
	})
}

func IsServiceUp(ctx context.Context, s *testing.State) {
	// Gather ModemManager properties
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Failed to create Modem (precondition): ", err)
	}
	modemProps, err := modem.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to call GetProperties on Modem: ", err)
	}
	simPath, err := modemProps.GetObjectPath(mmconst.ModemPropertySim)
	if err != nil {
		s.Fatal("Failed to get Modem.Sim property: ", err)
	}
	s.Log("SIM path = ", simPath)
	simProps, err := modem.GetSimProperties(ctx, simPath)
	if err != nil {
		s.Fatalf("Failed to create Sim for path: %q: %s", simPath, err)
	}
	simICCID, err := simProps.GetString(mmconst.SimPropertySimIdentifier)
	if err != nil {
		s.Fatal("Failed to get Sim.SimIdentifier property: ", err)
	}

	// Gather Shill Device properties
	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper")
	}
	deviceProps, err := helper.Device.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to get Device properties: ", err)
	}
	if simPresent, err := deviceProps.GetBool(shillconst.DevicePropertyCellularSIMPresent); err != nil {
		s.Fatal("Failed to get Device.Cellular.SIMPresent property: ", err)
	} else if !simPresent {
		s.Fatal("SIMPresent property not set")
	}
	deviceICCID, err := deviceProps.GetString(shillconst.DevicePropertyCellularICCID)
	if err != nil {
		s.Fatal("Failed to get Device.Cellular.ICCID property: ", err)
	}

	// Ensure Shill Device ICCID and ModemManager ICCID match.
	if deviceICCID != simICCID {
		s.Fatalf("Device ICCID does not match SIM, got %q, want %q", deviceICCID, simICCID)
	}

	// Ensure that Shill creates a Service matching ICCID.
	if _, err = helper.FindServiceForDevice(ctx); err != nil {
		s.Fatal("Failed to get Cellular Service for Device: ", err)
	}
}
