// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import "go.chromium.org/tast-tests/cros/common/firmware/ti50"

type gscDT struct {
}

func (g *gscDT) HasFpmcuUart() bool {
	return true
}

func (g *gscDT) ExpectedDidVidValue() []byte {
	return ti50.TpmTi50DidVidValue
}

func (g *gscDT) GscHostI2cBusses() map[byte]I2CBus {
	return map[byte]I2CBus{
		0: I2CBus{
			BusName:  ti50.I2cTi50Debug,
			DataPin:  ti50.GpioTi50I2cDbgSda,
			ClockPin: ti50.GpioTi50I2cDbgScl,
		},
		1: I2CBus{
			BusName:  ti50.I2cTi50Smbus,
			DataPin:  ti50.GpioTi50SmbusSda,
			ClockPin: ti50.GpioTi50SmbusScl,
		},
	}
}

func (g *gscDT) PreferredTPMBus() ti50.TpmBus {
	return ti50.TpmBusSpi
}
