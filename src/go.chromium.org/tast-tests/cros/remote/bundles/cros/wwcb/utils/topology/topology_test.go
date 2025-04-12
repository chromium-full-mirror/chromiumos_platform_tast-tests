// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package topology contains tools to interact with PASIT topology components.
package topology

import (
	"testing"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
)

func emptyTopology() *labapi.PasitHost {
	return &labapi.PasitHost{
		Devices:     []*labapi.PasitHost_Device{},
		Connections: []*labapi.PasitHost_Connection{},
	}
}

// pasitBoxDisplayTopology creates a display topology for pasit box.
func pasitBoxDisplayTopology(hostname, dockSwitch, dockHdmiSwitch, docklessDpSwitch string) *labapi.PasitHost {
	return &labapi.PasitHost{
		Devices: []*labapi.PasitHost_Device{
			{
				Id:   hostname,
				Type: labapi.PasitHost_Device_DUT,
			},
			{
				Id:   dockSwitch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   "dock_1",
				Type: labapi.PasitHost_Device_DOCKING_STATION,
			},
			{
				Id:   dockHdmiSwitch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   docklessDpSwitch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   "monitor_1",
				Type: labapi.PasitHost_Device_MONITOR,
			},
		},
		Connections: []*labapi.PasitHost_Connection{
			{
				Type:     "USBC",
				ParentId: hostname,
				ChildId:  dockSwitch,
			},
			{
				Type:     "DISPLAYPORT",
				ParentId: dockSwitch,
				ChildId:  docklessDpSwitch,
			},
			{
				Type:     "DISPLAYPORT",
				ParentId: docklessDpSwitch,
				ChildId:  "monitor_1",
			},
			{
				Type:     "USBC",
				ParentId: dockSwitch,
				ChildId:  "dock_1",
			},
			{
				Type:     "HDMI",
				ParentId: "dock_1",
				ChildId:  dockHdmiSwitch,
			},
			{
				Type:     "HDMI",
				ParentId: dockHdmiSwitch,
				ChildId:  "monitor_1",
			},
		},
	}
}

func verifyPath(t *testing.T, name string, got, want []string, err error, wantErr bool) {
	if (err != nil) != wantErr {
		t.Errorf("%s: error = %v, wantErr %v", name, err, wantErr)
		return
	}

	// if not expecting an error and expected path is empty, ignore the test case
	if len(want) == 0 {
		return
	}

	if len(got) != len(want) {
		t.Errorf("%s: invalid result, got %d devices in path expected %d", name, len(got), len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: invalid result, got %v want %v", name, got[i], want[i])
			return
		}
	}
}

// TODO(b/384765859): Add tests for switchService
func TestPaths(t *testing.T) {
	tests := []struct {
		name                 string
		topology             *labapi.PasitHost
		find                 devicePredicate
		expectedPath         []string
		pathNotFound         bool
		expectedDocklessPath []string
		docklessPathNotFound bool
	}{
		{
			name:                 "default_storage",
			topology:             DefaultStorageTopology("localhost", "storage_switch_id"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeStorage },
			expectedPath:         []string{"localhost", "storage_switch_id", "storage_1"},
			expectedDocklessPath: []string{"localhost", "storage_switch_id", "storage_1"},
		},
		{
			name:                 "default_camera",
			topology:             DefaultCameraTopology("localhost", "camera_switch_id"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeCamera },
			expectedPath:         []string{"localhost", "camera_switch_id", "camera_1"},
			expectedDocklessPath: []string{"localhost", "camera_switch_id", "camera_1"},
		},
		{
			name:                 "default_monitor.m1",
			topology:             DefaultDisplayTopology("localhost", "monitor_switch_1", "monitor_switch_2"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_1" },
			expectedPath:         []string{"localhost", "monitor_switch_1", "monitor_1"},
			expectedDocklessPath: []string{"localhost", "monitor_switch_1", "monitor_1"},
		},
		{
			name:                 "default_monitor.m2",
			topology:             DefaultDisplayTopology("localhost", "monitor_switch_1", "monitor_switch_2"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_2" },
			expectedPath:         []string{"localhost", "monitor_switch_1", "monitor_1", "monitor_switch_2", "monitor_2"},
			expectedDocklessPath: []string{"localhost", "monitor_switch_1", "monitor_1", "monitor_switch_2", "monitor_2"},
		},
		{
			name:                 "default_full.dock",
			topology:             DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeDockingStation },
			expectedPath:         []string{"localhost", "dock_switch", "dock_1"},
			docklessPathNotFound: true,
		},
		{
			name:                 "default_full.m1",
			topology:             DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_1" },
			expectedPath:         []string{"localhost", "dock_switch", "dock_1", "m1_switch", "monitor_1"},
			docklessPathNotFound: true,
		},
		{
			name:                 "default_full.m2",
			topology:             DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_2" },
			expectedPath:         []string{"localhost", "dock_switch", "dock_1", "m2_switch", "monitor_2"},
			docklessPathNotFound: true,
		},
		{
			name:                 "default_full.usb1_device",
			topology:             DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetId() == "device_1" },
			expectedPath:         []string{"localhost", "dock_switch", "dock_1", "usb1_switch", "device_1"},
			docklessPathNotFound: true,
		},
		{
			name:     "dockless_monitor.m1",
			topology: pasitBoxDisplayTopology("localhost", "dock_switch", "dock_monitor_switch", "dockless_monitor_switch"),
			find:     func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_1" },
			// expectedPath not deterministic, skipping the test case
			expectedDocklessPath: []string{"localhost", "dock_switch", "dockless_monitor_switch", "monitor_1"},
		},
		{
			name:                 "default_full.failure",
			topology:             DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:                 func(d *labapi.PasitHost_Device) bool { return d.GetId() == "unknown_id" },
			pathNotFound:         true,
			docklessPathNotFound: true,
		},
		{
			name:                 "empty_matching",
			topology:             emptyTopology(),
			find:                 func(d *labapi.PasitHost_Device) bool { return true },
			expectedPath:         []string{"localhost"},
			expectedDocklessPath: []string{"localhost"},
		},
		{
			name:                 "empty_non_matching",
			topology:             emptyTopology(),
			find:                 func(d *labapi.PasitHost_Device) bool { return false },
			pathNotFound:         true,
			docklessPathNotFound: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			helper := NewHelper(test.topology, "localhost", nil)

			path, err := helper.path(test.find, nil)
			verifyPath(t, "path", path, test.expectedPath, err, test.pathNotFound)

			ignore := func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeDockingStation }
			docklessPath, err := helper.path(test.find, ignore)
			verifyPath(t, "docklessPath", docklessPath, test.expectedDocklessPath, err, test.docklessPathNotFound)
		})
	}

}

func TestPathsVia(t *testing.T) {
	tests := []struct {
		name           string
		topology       *labapi.PasitHost
		via            devicePredicate
		find           labapi.PasitHost_Device_Type
		expectedResult []string
		wantErr        bool
	}{
		{
			name:           "default_full.m1_via_dock",
			topology:       DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			via:            func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeDockingStation },
			find:           DeviceTypeMonitor,
			expectedResult: []string{"localhost", "dock_switch", "dock_1", "m1_switch", "monitor_1"},
		},
		{
			name:           "default_display.m2_via_dock",
			topology:       DefaultDisplayTopology("localhost", "monitor_switch_1", "monitor_switch_2"),
			via:            func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeMonitor },
			find:           DeviceTypeMonitor,
			expectedResult: []string{"localhost", "monitor_switch_1", "monitor_1", "monitor_switch_2", "monitor_2"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			helper := NewHelper(test.topology, "localhost", nil)
			devices := helper.devicesByTypeVia(test.find, test.via)

			find := func(d *labapi.PasitHost_Device) bool { return d.GetId() == devices[0] }
			path, err := helper.path(find, nil)
			verifyPath(t, "path", path, test.expectedResult, err, test.wantErr)
		})
	}
}
