// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModemmanagerSlots,
		Desc:         "Verifies that modemmanager reports multiple SIM slots",
		Contacts:     []string{"chromeos-cellular-team@google.com", "cros-network-health-team@google.com", "ejcaruso@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_cq"},
		Fixture:      "cellularModemManager",
	})
}

// ModemmanagerSlots Test
func ModemmanagerSlots(ctx context.Context, s *testing.State) {
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		s.Fatal("Failed to create Modem (precondition): ", err)
	}
	props, err := modem.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to call GetProperties on Modem: ", err)
	}
	sim, err := props.GetObjectPath(mmconst.ModemPropertySim)
	if err != nil {
		s.Fatal("Missing Sim property: ", err)
	}
	simSlots, err := props.GetObjectPaths(mmconst.ModemPropertySimSlots)
	if err != nil {
		s.Fatal("Failed to get SimSlots property: ", err)
	}
	if len(simSlots) == 0 {
		s.Log("No SimSlots for device, ending test")
		return
	}
	primary, err := props.GetUint32(mmconst.ModemPropertyPrimarySimSlot)
	if err != nil {
		s.Fatal("Missing PrimarySimSlot property: ", err)
	}
	s.Log("Primary SIM slot: ", primary)
	if int(primary) > len(simSlots) {
		s.Fatal("Invalid PrimarySimSlot: ", primary)
	}
	if sim != simSlots[primary-1] {
		s.Fatalf("Sim property mismatch: %s", sim)
	}
}
