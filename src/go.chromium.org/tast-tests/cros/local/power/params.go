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

// ChargeParams defines parameters used for a charge test.
type ChargeParams struct {
	// Min battery percent used in a charge test.
	MinChargePercentage float64

	// Max battery percent used in a charge test.
	MaxChargePercentage float64

	// If set to true then AC power will be forced to be temporarily
	// disconnected to prevent battery from being charged.
	DischargeOnCompletion bool

	// Indicates if the charge test is customiezed or not; for a customized
	// charging test, both min and max percentage need to be provided via
	// `tast run -var="min_charge_percent=XX" -var=max_charge_percent=XX"`.
	IsCustomized bool

	// Indicates if the charge test is a power qual test; for a power qual
	// test to measure charging speed, the screen will be set to default
	// brightness instead of 0 for fast charging.
	IsPowerQual bool
}

// IdleParams defines the screen & bluetooth on/off behavior and the time params
// for a idle test.
type IdleParams struct {
	DisplayPower   bool
	BluetoothPower bool
	IdleTimeParams TimeParams
}
