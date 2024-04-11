// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	types "go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/networkui/netconfig"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           MojoGetDeviceStateListSimInfos,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Ensure that CrosNetworkConfig.DeviceStateProperties.sim_infos matches the properties in Shill ",
		Contacts:       []string{"chromeos-cellular-team@google.com", "cros-network-health-team@google.com", "khegde@google.com"},
		BugComponent:   "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps:   []string{"chrome"},
		Attr:           []string{"group:cellular", "cellular_sim_active"},
		Fixture:        "cellularE2ELocal",
		Timeout:        10 * time.Minute,
	})
}

func getCellularDevices(deviceState []types.DeviceStateProperties) []types.DeviceStateProperties {
	var result []types.DeviceStateProperties
	for _, d := range deviceState {
		if d.Type != types.Cellular {
			continue
		}
		result = append(result, d)
	}
	return result
}

func MojoGetDeviceStateListSimInfos(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(ctx)

	cellularHelper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	deviceProps, err := cellularHelper.Device.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to get Device properties: ", err)
	}

	info, err := deviceProps.Get(shillconst.DevicePropertyCellularSIMSlotInfo)
	if err != nil {
		s.Fatal("Failed to get Device.CellularSIMSlotInfo property: ", err)
	}
	simSlotInfo, ok := info.([]map[string]dbus.Variant)
	if !ok {
		s.Fatal("Invalid format for Device.CellularSIMSlotInfo")
	}

	deviceState, err := netConn.GetDeviceStateList(ctx)
	if err != nil {
		s.Fatal("Failed to get DeviceStateList: ", err)
	}

	devices := getCellularDevices(deviceState)
	if len(devices) != 1 {
		s.Fatal("Incorrect number of cellular devices, want: 1, got: ", len(devices))
	}
	cellular := devices[0]

	if len(cellular.SimInfos) != len(simSlotInfo) {
		s.Fatalf("Mismatch simSlotInfo lengths, want: %v, got: %v", len(simSlotInfo), len(cellular.SimInfos))
	}
	for id, slot := range simSlotInfo {
		if cellular.SimInfos[id].SlotID != int32(id+1) {
			s.Fatalf("SlotID mismatch, got: %v, want: %v", cellular.SimInfos[id].SlotID, id+1)
		}
		primary, err := netconfig.GetBoolValue(slot, "Primary")
		if err != nil {
			s.Fatal("Failed to get primary value: ", err)
		}
		if cellular.SimInfos[id].IsPrimary != primary {
			s.Fatalf("Primary field mismatch, want: %v, got: %v", primary, cellular.SimInfos[id])
		}
		// Shill values are not populated for non-active slots. Chrome
		// gets the values from Hermes directly so they will be
		// populated.
		if !cellular.SimInfos[id].IsPrimary {
			continue
		}
		eid, err := netconfig.GetStringValue(slot, "EID")
		if err != nil {
			s.Fatal("Failed to get eid: ", err)
		}
		if cellular.SimInfos[id].Eid != eid {
			s.Fatalf("EID field mismatch, want: %v, got: %v", eid, cellular.SimInfos[id])
		}
		iccid, err := netconfig.GetStringValue(slot, "ICCID")
		if err != nil {
			s.Fatal("Failed to get iccid: ", err)
		}
		if cellular.SimInfos[id].Iccid != iccid {
			s.Fatalf("ICCID field mismatch, want: %v, got: %v", iccid, cellular.SimInfos[id])
		}
	}
}
