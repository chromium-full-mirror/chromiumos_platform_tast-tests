// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/testing/cellularconst"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IsModemFirmwareKnown,
		Desc:         "Verifies that the modem FW version is known",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active"},
	})
}

// IsModemFirmwareKnown Test
func IsModemFirmwareKnown(ctx context.Context, s *testing.State) {
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		s.Fatal("Failed to create modem: ", err)
	}
	fwVersion, err := modem.GetFwVersion(ctx, modem)
	if err != nil {
		s.Fatal("Failed to get FW version: ", err)
	}
	testing.ContextLog(ctx, "FW version: ", fwVersion)
	modemType, err := cellular.GetModemType(ctx)
	if err != nil {
		s.Fatal("Failed to get modem type: ", err)
	}
	switch modemType {
	case cellularconst.ModemTypeEM060:
		if cellular.ModemFwFilterEM060V01.IsMatch(modemType, fwVersion) {
			return
		}
	case cellularconst.ModemTypeFM101:
		if cellular.ModemFwFilterFM101MR1.IsMatch(modemType, fwVersion) || cellular.ModemFwFilterFM101MR2.IsMatch(modemType, fwVersion) {
			return
		}
	case cellularconst.ModemTypeFM350:
		if cellular.ModemFwFilterFM350MR3AndLower.IsMatch(modemType, fwVersion) || cellular.ModemFwFilterFM350MR4.IsMatch(modemType, fwVersion) {
			return
		}
	case cellularconst.ModemTypeL850:
		if cellular.ModemFwFilterL850MR8AndLower.IsMatch(modemType, fwVersion) {
			return
		}
	case cellularconst.ModemTypeNL668:
		if cellular.ModemFwFilterNL668A01.IsMatch(modemType, fwVersion) || cellular.ModemFwFilterNL668A04.IsMatch(modemType, fwVersion) {
			return
		}
	case cellularconst.ModemTypeSC7180:
		if cellular.ModemFwFilterSC7180All.IsMatch(modemType, fwVersion) {
			return
		}
	}
	s.Fatalf("Unknown FW version %s on modem: %s", fwVersion, modemType.String())

}
