// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vpd contains functions to read and write VPD flash memory.
package vpd

import (
	"os"
	"path/filepath"

	"go.chromium.org/tast/core/errors"
)

const vpdCacheFolder = "/sys/firmware/vpd/"

var activateDateFileName = filepath.Join(vpdCacheFolder, "rw/ActivateDate")
var fsiVersionFileName = filepath.Join(vpdCacheFolder, "ro/FsiVersion")

// ActivateDate reads the ActivateDate as stored in vpd.
// Returns nil and no error if the value is not set in vpd.
// Note that this reads cached values populated on boot, you would need to reboot to propagate changes.
func ActivateDate() (*string, error) {
	data, err := os.ReadFile(activateDateFileName)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "failed to read %s", activateDateFileName)
	}
	if os.IsNotExist(err) {
		return nil, nil
	}
	dataString := string(data)
	return &dataString, nil
}

// FsiVersion read fsi_version as stored in vpd. Note that this is only present for new devices.
// Returns nil and no error if the value is not set in vpd.
// Note that this reads cached values populated on boot, you would need to reboot to propagate changes.
func FsiVersion() (*string, error) {
	data, err := os.ReadFile(fsiVersionFileName)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "failed to read %s", fsiVersionFileName)
	}
	if os.IsNotExist(err) {
		return nil, nil
	}
	dataString := string(data)
	return &dataString, nil
}
