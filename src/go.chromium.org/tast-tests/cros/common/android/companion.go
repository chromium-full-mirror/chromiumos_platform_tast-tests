// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package android

import (
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CompanionsVarName defines the global runtime variable name for
// Android companion devices.
const CompanionsVarName = "android.companions"

var (
	// companionsVar is a runtime variable for storing information of
	// android companion devices
	//
	// The format of the value should be:
	// device_list := <device> | <device>,[device_list]
	// device := <model>:<associated_hostname>:<serial_number>
	//
	// Exammple:
	//    chromeos15-row9-metro5-labstation2:1C291FDEE00923:pixel5a
	companionsVar = testing.RegisterVarString(
		CompanionsVarName,
		"",
		"Android companion devices information")
)

// Companion stores device information of an Android companion device.
type Companion struct {
	AssociatedHostname string
	SerialNumber       string
	ModelName          string
}

// Companions return a list of Android companion devices that can be used for testing.
func Companions() (companions []Companion, err error) {
	rawValue := companionsVar.Value()
	devices := strings.Split(rawValue, ",")
	for _, d := range devices {
		deviceInfo := strings.Split(d, ":")
		if len(deviceInfo) < 3 {
			return nil, errors.Errorf("syntax error in Android companion information: %q", d)
		}
		// We may have a port forwarding address (e.g. localhost:2222) or
		// IPV6 address (e.g. 2001:db8:3333:4444:5555:6666:7777:8888) for companion host.
		if len(deviceInfo) > 3 {
			deviceInfo[0] = strings.Join(deviceInfo[0:len(deviceInfo)-2], ":")
			deviceInfo[1] = deviceInfo[len(deviceInfo)-2]
			deviceInfo[2] = deviceInfo[len(deviceInfo)-1]
		}
		companions = append(companions, Companion{
			AssociatedHostname: deviceInfo[0],
			SerialNumber:       deviceInfo[1],
			ModelName:          deviceInfo[2],
		})
	}
	return companions, nil
}
