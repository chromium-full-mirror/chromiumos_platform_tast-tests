// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import "time"

// TimeParams defines the time interval used in power.NewRecorder
// and the total sleep duration in each power test.
type TimeParams struct {
	Interval time.Duration
	Total    time.Duration
}

// ChargeParams defines the min and max charge percent used in charge
// test. It also includes two boolean variables.
// DischargeOnCompletion: if set to true then AC power will be forced to
// be temporarily disconnected to prevent battery from being charged
// Customized: indicates if the charge test is customiezed or not
type ChargeParams struct {
	MinChargePercentage   float64
	MaxChargePercentage   float64
	DischargeOnCompletion bool
	Customized            bool
}
