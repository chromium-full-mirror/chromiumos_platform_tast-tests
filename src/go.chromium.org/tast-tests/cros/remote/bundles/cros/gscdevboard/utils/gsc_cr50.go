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

func (g *gscCr50) GscHostI2cBusses() map[byte]I2CBus {
	return map[byte]I2CBus{
		0: I2CBus{
			BusName:  ti50.I2cTi50Debug,
			DataPin:  ti50.GpioTi50I2cDbgSda,
			ClockPin: ti50.GpioTi50I2cDbgScl,
		},
	}
}

func (g *gscCr50) PreferredTPMBus() ti50.TpmBus {
	return ti50.TpmBusSpi
}

func (g *gscCr50) HasEcRstFet() bool {
	// No EC Reset FET pin, but designs with Cr50 also have double reset issue.
	return false
}
