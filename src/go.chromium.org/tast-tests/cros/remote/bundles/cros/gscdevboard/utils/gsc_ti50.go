// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import "go.chromium.org/tast-tests/cros/common/firmware/ti50"

type gscTi50 struct {
}

func (g *gscTi50) HasFpmcuUart() bool {
	return true
}

func (g *gscTi50) ExpectedDidVidValue() []byte {
	return ti50.TpmTi50DidVidValue
}

func (g *gscTi50) GscHostI2cBusses() map[byte]ti50.I2cBusName {
	return map[byte]ti50.I2cBusName{
		0: ti50.I2cTi50Debug,
		1: ti50.I2cTi50Smbus,
	}
}
