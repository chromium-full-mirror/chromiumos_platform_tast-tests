// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package topology contains tools to interact with PASIT topology components.
package topology

import (
	"testing"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
)

func TestPaths(t *testing.T) {
	tests := []struct {
		name           string
		topology       *labapi.PasitHost
		find           devicePredicate
		expectedResult []string
		wantErr        bool
	}{
		{
			name:           "default_storage",
			topology:       DefaultStorageTopology("localhost", "storage_switch_id"),
			find:           func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeStorage },
			expectedResult: []string{"localhost", "storage_switch_id", "storage_1"},
		},
		{
			name:           "default_camera",
			topology:       DefaultCameraTopology("localhost", "camera_switch_id"),
			find:           func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeCamera },
			expectedResult: []string{"localhost", "camera_switch_id", "camera_1"},
		},
		{
			name:           "default_monitor.m1",
			topology:       DefaultDisplayTopology("localhost", "monitor_switch_1", "monitor_switch_2"),
			find:           func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_1" },
			expectedResult: []string{"localhost", "monitor_switch_1", "monitor_1"},
		},
		{
			name:           "default_monitor.m2",
			topology:       DefaultDisplayTopology("localhost", "monitor_switch_1", "monitor_switch_2"),
			find:           func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_2" },
			expectedResult: []string{"localhost", "monitor_switch_1", "monitor_1", "monitor_switch_2", "monitor_2"},
		},
		{
			name:           "default_full.dock",
			topology:       DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:           func(d *labapi.PasitHost_Device) bool { return d.GetType() == DeviceTypeDockingStation },
			expectedResult: []string{"localhost", "dock_switch", "dock_1"},
		},
		{
			name:           "default_full.m1",
			topology:       DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:           func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_1" },
			expectedResult: []string{"localhost", "dock_switch", "dock_1", "m1_switch", "monitor_1"},
		},
		{
			name:           "default_full.m2",
			topology:       DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:           func(d *labapi.PasitHost_Device) bool { return d.GetId() == "monitor_2" },
			expectedResult: []string{"localhost", "dock_switch", "dock_1", "m2_switch", "monitor_2"},
		},
		{
			name:           "default_full.usb1_device",
			topology:       DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:           func(d *labapi.PasitHost_Device) bool { return d.GetId() == "device_1" },
			expectedResult: []string{"localhost", "dock_switch", "dock_1", "usb1_switch", "device_1"},
		},
		{
			name:     "default_full.failure",
			topology: DefaultFullTopology("localhost", "dock_switch", "m1_switch", "m2_switch", "eth_switch", "usb1_switch"),
			find:     func(d *labapi.PasitHost_Device) bool { return d.GetId() == "unknown_id" },
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			helper := NewHelper(test.topology, "localhost")
			path, err := helper.path(test.find)
			if len(path) != len(test.expectedResult) {
				t.Errorf("invalid result, got %d devices in path expected %d", len(path), len(test.expectedResult))
				return
			}
			for i := range test.expectedResult {
				if path[i] != test.expectedResult[i] {
					t.Errorf("invalid result, got %v expected %v", path[i], test.expectedResult[i])
					return
				}
			}
			if (err != nil) != test.wantErr {
				t.Errorf("error = %v, wantErr %v", err, test.wantErr)
				return
			}
		})
	}

}
