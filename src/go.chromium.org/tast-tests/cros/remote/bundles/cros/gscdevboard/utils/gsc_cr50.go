// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import "go.chromium.org/tast-tests/cros/common/firmware/ti50"

type gscCr50 struct {
}

func (g *gscCr50) HasFpmcuUart() bool {
	return false
}

func (g *gscCr50) ExpectedDidVidValue() []byte {
	return ti50.TpmCr50DidVidValue
}

func (g *gscCr50) GscHostI2cBusses() map[byte]ti50.I2cBusName {
	return map[byte]ti50.I2cBusName{
		0: ti50.I2cTi50Debug,
		1: ti50.I2cTi50Smbus,
	}
}
