// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usb

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/services/cros/usb"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	deviceDir = "/sys/bus/usb/devices"

	// Regex to match USB device names in /sys/bus/usb/devices.
	// Interfaces and root hubs will not match.
	deviceRegex = "[0-9]+-[0-9.]+$"
)

type SysfsService struct {
	s *testing.ServiceState
}

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			usb.RegisterSysfsServiceServer(srv, &SysfsService{s: s})
		},
	})
}

// readDeviceAttributes reads a single USB device's properties based on the device's address.
func readDeviceAttributes(deviceAddr string) (*usb.Device, error) {
	// If the device does not exist in sysfs, return an error.
	_, err := os.Stat(filepath.Join(deviceDir, deviceAddr))
	if err != nil {
		return nil, errors.New("unable to access device at " + deviceAddr)
	}

	// Try to read the device properties.
	// This is best effort. Not all USB devices will have the same properties.
	var device usb.Device
	if f, err := os.ReadFile(filepath.Join(deviceDir, deviceAddr, "devnum")); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(f)), 10, 64); err == nil {
			device.Devnum = val
		}
	}

	if f, err := os.ReadFile(filepath.Join(deviceDir, deviceAddr, "speed")); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(f)), 10, 64); err == nil {
			device.Speed = val
		}
	}

	if f, err := os.ReadFile(filepath.Join(deviceDir, deviceAddr, "removable")); err == nil {
		switch strings.TrimSpace(string((f))) {
		case "unknown":
			device.Removable = usb.RemovableAttribute_REMOVABLE_ATTRIBUTE_UNKNOWN
		case "fixed":
			device.Removable = usb.RemovableAttribute_REMOVABLE_ATTRIBUTE_FIXED
		case "removable":
			device.Removable = usb.RemovableAttribute_REMOVABLE_ATTRIBUTE_REMOVABLE
		}
	}

	return &device, nil
}

// buildDeviceMap returns a string to usb.Device map based on the usb.Device struct defined by the service protocol.
// The map's keys will be device directory name in sysfs, which is the busnum followed by the devpath.
func buildDeviceMap() (map[string]*usb.Device, error) {
	deviceMap := map[string]*usb.Device{}

	// Get list of paths at /sys/bus/usb/devices.
	devicePaths, err := os.ReadDir(deviceDir)
	if err != nil {
		return deviceMap, errors.New("unable to access USB directory")
	}

	deviceRegex := regexp.MustCompile(deviceRegex)
	for _, p := range devicePaths {
		// Check path is a device.
		if !deviceRegex.MatchString(p.Name()) {
			continue
		}

		// Read device attributes into the device map.
		// If readDeviceAttributes hits an error, continue to the next device.
		device, err := readDeviceAttributes(p.Name())
		if err == nil {
			deviceMap[p.Name()] = device
		}
	}

	return deviceMap, nil
}

// GetDevices will return the current USB device state based on the device struct defined in the service protocol.
func (s *SysfsService) GetDevices(ctx context.Context, req *empty.Empty) (*usb.DeviceMap, error) {
	devices, err := buildDeviceMap()
	if err != nil {
		return &usb.DeviceMap{Devices: map[string]*usb.Device{}}, errors.Wrap(err, "unable to build device map")
	}

	return &usb.DeviceMap{Devices: devices}, nil
}
