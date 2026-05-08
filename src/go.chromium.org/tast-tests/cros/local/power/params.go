// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/display"
)

// TimeParams defines the time interval used in power.NewRecorder
// and the total sleep duration in each power test.
type TimeParams struct {
	Interval time.Duration
	Total    time.Duration
}

// ChargeParams defines parameters used for a charge test.
type ChargeParams struct {
	// Max battery battery preparation time.
	MaxBatteryPreparationTime time.Duration

	// Min battery percent used in a charge test.
	MinChargePercentage float64

	// Max battery percent used in a charge test.
	MaxChargePercentage float64

	// UseDisplayPercentage indicates the requested charge percentage is battery
	// display percentage. If not set, the percentage is battery percentage.
	UseDisplayPercentage bool

	// If set to true then AC power will be forced to be temporarily
	// disconnected via "ectool" to prevent battery from being charged.
	DischargeOnCompletion bool

	// Indicates if the charge test is customiezed or not; for a customized
	// charging test, both min and max percentage need to be provided via
	// `tast run -var="min_charge_percent=XX" -var="max_charge_percent=XX"`.
	IsCustomized bool

	// Indicates if the charge test is a power qual test; for a power qual
	// test to measure charging speed, the screen will be set to default
	// brightness instead of 0 for fast charging.
	IsPowerQual bool

	// EnableDischargeWatchdog indicates whether to enable the discharge watchdog
	// in drainBattery to reliably discharge the battery despite software glitches.
	EnableDischargeWatchdog bool

	// SkipDegradationAdjustment indicates whether to skip adjusting the charge
	// percentage to account for battery degradation.
	// If true, MinChargePercentage is interpreted as a percentage of the
	// battery's current full capacity.
	// If false, MinChargePercentage is interpreted as a percentage of the
	// battery's design capacity; the actual charge target is then adjusted and
	// capped at 80% to avoid long charge times.
	SkipDegradationAdjustment bool
}

// RegressionTestChargeParam is used for tests in power_regression suite that
// take >= 1hr. They need to be charged to 50% of battery to prevent force
// discharge failure from low battery threshold.
var RegressionTestChargeParam = ChargeParams{
	MaxBatteryPreparationTime: 2 * time.Hour,
	MinChargePercentage:       50.0,
	MaxChargePercentage:       100.0,
	DischargeOnCompletion:     true,
	IsCustomized:              false,
	IsPowerQual:               false,
}

// IdleParams defines the screen & bluetooth on/off behavior and the time params
// for a idle test.
type IdleParams struct {
	DisplayPower   bool
	BluetoothPower bool
	CollectTrace   bool
	PSRState       display.PSRState
	IdleTimeParams TimeParams
}

// VideoEncodeFormatParams defines video encoding format.
type VideoEncodeFormatParams struct {
	// Video coding format, such as vp9, vp8, h264, av1, etc.
	Codec string
	// Display of resolution, such as fd, hfd, vga, qvga, hvga, qhvgs, etc.
	Resolution string
	// Framerate of video content.
	Framerate int64

	VideoEncodeTimeParams TimeParams
}

// DisplayParams defines the screen brightness, display pages and time params
// for a display test.
type DisplayParams struct {
	// If Brightness is set to "all_brightness", then power consumption will be collected at
	// every UI brightness level.
	Brightness        string
	Pages             []string
	DataDir           string
	DisplayTimeParams TimeParams
}
