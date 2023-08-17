// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package networkrequestmonitor contains values used across the umbrella test and its component subtests.
package networkrequestmonitor

// PolicySetting is the key for a test case of a service, indicating the policy
// value being tested and other expectations of the service behavior.
type PolicySetting int

const (
	// PolicyEnabled is for enabled service test cases.
	PolicyEnabled PolicySetting = iota

	// PolicyDisabled is for disabled service test cases.
	PolicyDisabled

	// PolicyUnset is for the default unset state of the service test case.
	PolicyUnset
)
