// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import "go.chromium.org/tast-tests/cros/common/firmware/ti50"

// GscProperties advertises various aspects of the GSC currently under test.
type GscProperties interface {
	// HasFpmcuUart indicates whether the GSC has a third UART connected to the FPMCU.
	HasFpmcuUart() bool
	// ExpectedDidVidValue returns the value expected when reading the TPM DID_VID register.
	ExpectedDidVidValue() []byte
	// GscHostI2cBusses returns the set of I2C busses on which the GSC can act as host, the
	// map key is the port value to be used when tunneling requests through CCD.
	GscHostI2cBusses() map[byte]ti50.I2cBusName
}
