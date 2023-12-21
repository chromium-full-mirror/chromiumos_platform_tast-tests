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
	// Regex to match USB interfaces within a device directory.
	interfRegex = "[0-9]+-[0-9.]+:[0-9.]+$"
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

// readInterfaceAttributes reads a USB interface's properties from its location in sysfs.
func readInterfaceAttributes(ss *testing.ServiceState, deviceAddr, interfAddr string) *usb.Interface {
	// Try to read the interface properties.
	var interf usb.Interface
	if f, err := os.ReadFile(filepath.Join(deviceDir, deviceAddr, interfAddr, "bInterfaceClass")); err != nil {
		ss.Logf("Warning: unable to find interface class file for %s: %s", interfAddr, err.Error())
	} else {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(f)), 16, 64); err != nil {
			ss.Logf("Warning: unable to read interface class for %s: %s", interfAddr, err.Error())
		} else {
			interf.InterfaceClass = val
		}
	}

	return &interf
}

// buildInterfaceMap returns a string to usb.Interface map based on the usb.Interface struct defined by the service protocol.
func buildInterfaceMap(ss *testing.ServiceState, deviceAddr string) (map[string]*usb.Interface, error) {
	interfMap := map[string]*usb.Interface{}

	// Get list of interface paths from device dir.
	interfPaths, err := os.ReadDir(filepath.Join(deviceDir, deviceAddr))
	if err != nil {
		return interfMap, errors.Wrap(err, "unable to access device directory")
	}

	interfRegex := regexp.MustCompile(interfRegex)
	for _, p := range interfPaths {
		// Check path is an interface.
		if !interfRegex.MatchString(p.Name()) {
			continue
		}

		interfMap[p.Name()] = readInterfaceAttributes(ss, deviceAddr, p.Name())
	}

	return interfMap, nil
}

// readDeviceAttributes reads a single USB device's properties based on the device's address.
func readDeviceAttributes(ss *testing.ServiceState, deviceAddr string) (*usb.Device, error) {
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

	if f, err := os.ReadFile(filepath.Join(deviceDir, deviceAddr, "bDeviceClass")); err != nil {
		ss.Logf("Warning: unable to find device class file for %s: %s", deviceAddr, err.Error())
	} else {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(f)), 16, 64); err != nil {
			ss.Logf("Warning: unable to read device class for %s: %s", deviceAddr, err.Error())
		} else {
			device.DeviceClass = val
		}
	}

	if interfMap, err := buildInterfaceMap(ss, deviceAddr); err != nil {
		ss.Logf("Warning: unable to build interface map: %s", err.Error())
	} else {
		device.Interfaces = interfMap
	}

	return &device, nil
}

// buildDeviceMap returns a string to usb.Device map based on the usb.Device struct defined by the service protocol.
// The map's keys will be device directory name in sysfs, which is the busnum followed by the devpath.
func buildDeviceMap(ss *testing.ServiceState) (map[string]*usb.Device, error) {
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
		device, err := readDeviceAttributes(ss, p.Name())
		if err == nil {
			deviceMap[p.Name()] = device
		}
	}

	return deviceMap, nil
}

// GetDevices will return the current USB device state based on the device struct defined in the service protocol.
func (s *SysfsService) GetDevices(ctx context.Context, req *empty.Empty) (*usb.DeviceMap, error) {
	devices, err := buildDeviceMap(s.s)
	if err != nil {
		return &usb.DeviceMap{Devices: map[string]*usb.Device{}}, errors.Wrap(err, "unable to build device map")
	}

	return &usb.DeviceMap{Devices: devices}, nil
}
