// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"io/ioutil"
	"path"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ThermalState reflects the thermal situation on device. Please keep it
// consistent with `power_manager::system::ThermalState`:
//
//	http://shortn/_WV2b4FriCw
type ThermalState int

// Levels of thermal state. See http://shortn/_WV2b4FriCw.
const (
	ThermalStateUnknown ThermalState = iota
	ThermalStateNominal
	ThermalStateFair
	ThermalStateSerious
	ThermalStateCritical
)

// ListingSysfsCoolingDevices returns the list of cooling devices.
func ListingSysfsCoolingDevices(ctx context.Context) ([]*CoolingDevice, error) {
	var devices []*CoolingDevice
	const sysfsThermalPath = "/sys/class/thermal"
	testing.ContextLog(ctx, "Listing cooling devices in ", sysfsThermalPath)
	files, err := ioutil.ReadDir(sysfsThermalPath)
	if err != nil {
		return devices, errors.Wrap(err, "failed to read sysfs dir")
	}

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "cooling_device") {
			continue
		}

		devPath := path.Join(sysfsThermalPath, file.Name())
		device, err := NewCoolingDevice(ctx, devPath)
		if err != nil {
			testing.ContextLog(ctx, "Failed to create CoolingDevice for ", devPath)
			continue
		}

		devices = append(devices, device)
	}

	return devices, nil
}
