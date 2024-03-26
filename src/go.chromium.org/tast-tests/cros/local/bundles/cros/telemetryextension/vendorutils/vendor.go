// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vendorutils contains utils to fetch OEM info.
package vendorutils

import (
	"context"
	"os"

	"go.chromium.org/tast-tests/cros/local/crosconfig"

	"go.chromium.org/tast/core/errors"
)

// FetchVendor returns vendor name using the CrOSConfig based approach and the vpd approach as a backup.
func FetchVendor(ctx context.Context) (string, error) {
	if got, err := crosconfig.Get(ctx, "/branding", "oem-name"); err != nil && !crosconfig.IsNotFound(err) {
		return "", errors.Wrap(err, "failed to get OEM name from CrOSConfig")
	} else if err == nil {
		return got, nil
	}

	if got, err := os.ReadFile("/sys/firmware/vpd/ro/oem_name"); err != nil && !os.IsNotExist(err) {
		return "", errors.Wrap(err, "failed to get OEM name from VPD field")
	} else if err == nil {
		return string(got), nil
	}

	return "", nil
}
