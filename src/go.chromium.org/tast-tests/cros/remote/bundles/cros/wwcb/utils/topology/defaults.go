// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package topology contains tools to interact with PASIT topology components.
package topology

import (
	"fmt"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
)

// DefaultFullTopology creates a default "pasit_full" topology with the provided IDs.
func DefaultFullTopology(hostname, dockSwitch, m1Switch, m2Switch, ethSwitch string, auxiliary ...string) *labapi.PasitHost {
	topology := &labapi.PasitHost{
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
				Id:   m1Switch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   "monitor_1",
				Type: labapi.PasitHost_Device_MONITOR,
			},
			{
				Id:   m2Switch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   "monitor_2",
				Type: labapi.PasitHost_Device_MONITOR,
			},
			{
				Id:   ethSwitch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   "network_1",
				Type: labapi.PasitHost_Device_NETWORK,
			},
		},
		Connections: []*labapi.PasitHost_Connection{
			{
				Type:     "USBC",
				ParentId: hostname,
				ChildId:  dockSwitch,
			},
			{
				Type:     "USBC",
				ParentId: dockSwitch,
				ChildId:  "dock_1",
			},
			{
				Type:     "HDMI",
				ParentId: "dock_1",
				ChildId:  m1Switch,
			},
			{
				Type:     "HDMI",
				ParentId: "dock_1",
				ChildId:  m2Switch,
			},
			{
				Type:     "HDMI",
				ParentId: m1Switch,
				ChildId:  "monitor_1",
			},
			{
				Type:     "HDMI",
				ParentId: m2Switch,
				ChildId:  "monitor_2",
			},
			{
				Type:     "ETHERNET",
				ParentId: "dock_1",
				ChildId:  ethSwitch,
			},
			{
				Type:     "ETHERNET",
				ParentId: ethSwitch,
				ChildId:  "network_1",
			},
		},
	}

	// Append all "auxiliary" devices connected to the dock.
	for i, switchID := range auxiliary {
		device := fmt.Sprintf("device_%d", i+1)
		topology.Devices = append(topology.Devices, &labapi.PasitHost_Device{
			Id:   switchID,
			Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
		})
		topology.Devices = append(topology.Devices, &labapi.PasitHost_Device{
			Id:   device,
			Type: labapi.PasitHost_Device_HID,
		})
		topology.Connections = append(topology.Connections, &labapi.PasitHost_Connection{
			Type:     "USBA",
			ParentId: "dock_1",
			ChildId:  switchID,
		})
		topology.Connections = append(topology.Connections, &labapi.PasitHost_Connection{
			Type:     "USBA",
			ParentId: switchID,
			ChildId:  device,
		})
	}

	return topology
}

// DefaultDisplayTopology creates a default topology for simple display tests.
func DefaultDisplayTopology(hostname, m1Switch, m2Switch string) *labapi.PasitHost {
	return &labapi.PasitHost{
		Devices: []*labapi.PasitHost_Device{
			{
				Id:   hostname,
				Type: labapi.PasitHost_Device_DUT,
			},
			{
				Id:   m1Switch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   m2Switch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   "monitor_1",
				Type: labapi.PasitHost_Device_MONITOR,
			},
			{
				Id:   "monitor_2",
				Type: labapi.PasitHost_Device_MONITOR,
			},
		},
		Connections: []*labapi.PasitHost_Connection{
			{
				Type:     "USBC",
				ParentId: hostname,
				ChildId:  m1Switch,
			},
			{
				Type:     "DISPLAYPORT",
				ParentId: m1Switch,
				ChildId:  "monitor_1",
			},
			{
				Type:     "DISPLAYPORT",
				ParentId: "monitor_1",
				ChildId:  m2Switch,
			},
			{
				Type:     "DISPLAYPORT",
				ParentId: m2Switch,
				ChildId:  "monitor_2",
			},
		},
	}
}

// DefaultStorageTopology creates a limited topology just for basic storage tests.
func DefaultStorageTopology(hostname, storageSwitch string) *labapi.PasitHost {
	return &labapi.PasitHost{
		Devices: []*labapi.PasitHost_Device{
			{
				Id:   hostname,
				Type: labapi.PasitHost_Device_DUT,
			},
			{
				Id:   storageSwitch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   "storage_1",
				Type: labapi.PasitHost_Device_STORAGE,
			},
		},
		Connections: []*labapi.PasitHost_Connection{
			{
				Type:     "USBA",
				ParentId: hostname,
				ChildId:  storageSwitch,
			},
			{
				Type:     "USBA",
				ParentId: storageSwitch,
				ChildId:  "storage_1",
			},
		},
	}
}

// DefaultCameraTopology creates a limited topology just for basic camera tests.
func DefaultCameraTopology(hostname, cameraSwitch string) *labapi.PasitHost {
	return &labapi.PasitHost{
		Devices: []*labapi.PasitHost_Device{
			{
				Id:   hostname,
				Type: labapi.PasitHost_Device_DUT,
			},
			{
				Id:   cameraSwitch,
				Type: labapi.PasitHost_Device_SWITCH_FIXTURE,
			},
			{
				Id:   "camera_1",
				Type: labapi.PasitHost_Device_CAMERA,
			},
		},
		Connections: []*labapi.PasitHost_Connection{
			{
				Type:     "USBA",
				ParentId: hostname,
				ChildId:  cameraSwitch,
			},
			{
				Type:     "USBA",
				ParentId: cameraSwitch,
				ChildId:  "camera_1",
			},
		},
	}
}
