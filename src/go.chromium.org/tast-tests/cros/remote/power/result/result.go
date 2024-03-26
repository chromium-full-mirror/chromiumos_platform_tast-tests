// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package result defines the power qual run result.
package result

// FormatVersion is the current result format version.
const FormatVersion int32 = 1

// Average contains the average power test result.
type Average struct {
	MinutesBatteryLife       float64 `json:"minutes_battery_life"`
	MinutesBatteryLifeTested float64 `json:"minutes_battery_life_tested"`
	DischargeRate            float64 `json:"watt_discharge_rate"`
}

// Power contains the power test result.
type Power struct {
	Average Average `json:"average"`
}

// Test has the result for a test in a power test persona.
type Test struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Power  Power   `json:"power"`
}

// Persona has the result for one power test persona.
type Persona struct {
	Name    string   `json:"name"`
	Power   Power    `json:"power"`
	Skipped []string `json:"skipped_tests"`
	Tests   []Test   `json:"tests"`
}

// Result defines the power qual test result.
type Result struct {
	FormatVersion int32     `json:"format_version"`
	Name          string    `json:"config_name"`
	Version       string    `json:"config_version"`
	Personas      []Persona `json:"persona"`
}
