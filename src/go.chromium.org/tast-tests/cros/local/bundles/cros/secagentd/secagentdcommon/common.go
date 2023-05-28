// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentdcommon contains shared general helpers used in secagentd tast tests.
package secagentdcommon

import (
	xdr "chromiumos/xdr/secagentd"

	"go.chromium.org/tast/core/errors"
)

// CheckCommon verifies that the common message fields are filled with appropriate values.
func CheckCommon(common *xdr.CommonEventVariantDataFields) error {
	if common.GetCreateTimestampUs() == 0 {
		return errors.New("CreateTimestampUs field not set")
	}
	deviceUser := common.GetDeviceUser()
	if common.DeviceUser == nil || deviceUser == "Unknown" {
		return errors.Errorf("invalid username: %s", deviceUser)
	}

	return nil
}
