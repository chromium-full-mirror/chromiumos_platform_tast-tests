// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tape

import (
	"context"
	"io/ioutil"
	"regexp"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// GetDeviceIDHelper retrieves the device id from the /var/lib/devicesettings/policy.1 file.
func GetDeviceIDHelper(ctx context.Context) (deviceID, customerID string, retErr error) {
	const deviceSettingsFileName = "/var/lib/devicesettings/policy.1"

	data, err := ioutil.ReadFile(deviceSettingsFileName)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to read %s", deviceSettingsFileName)
	}
	deviceSettings := strings.ToValidUTF8(string(data), "")

	// The deviceID is prefixed with $ and has the format xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx and is separated by a
	// SOH(x01) character and a tab from the customerID which has the format Cxxxxxxxx.
	r, err := regexp.Compile("\\$([a-z0-9]){8}-([a-z0-9]){4}-([a-z0-9]){4}-([a-z0-9]){4}-([a-z0-9]){12}\x01\tC([a-z]|[0-9]){8}")
	if err != nil {
		return "", "", errors.Wrap(err, "failed to compile regular expression")
	}

	deviceAndCustomerID := strings.Split(r.FindString(deviceSettings)[1:], "\x01\t")
	if len(deviceAndCustomerID) != 2 {
		return "", "", errors.New("failed to find device and customerID in devicesettings")
	}

	return deviceAndCustomerID[0], deviceAndCustomerID[1], nil
}
