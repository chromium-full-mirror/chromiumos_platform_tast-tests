// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cujrecorder

import "go.chromium.org/tast/core/testing"

// keepWifi forces the Wifi to remain in its initial state,
// regardless of the options passed to the Recorder. Useful for
// local testing if Wifi is used for the SSH connection to the DUT.
var keepWifi = testing.RegisterVarString(
	"cujrecorder.keepWifi",
	"",
	"A boolean string (true/false) signifying whether to force skipping disabling Wifi for the Recorder",
)

// skipPowerTest skips the power test, which allows for the device to
// charge and keeps Wifi in its initial state, regardless of the
// options passed to the Recorder.
var skipPowerTest = testing.RegisterVarString(
	"cujrecorder.skipPowerTest",
	"",
	"A boolean string (true/false) signifying whether to skip the power test for the Recorder",
)

// keepCharging keeps the test device charging during the test.
var keepCharging = testing.RegisterVarString(
	"cujrecorder.keepCharging",
	"",
	"A boolean string (true/false) signifying whether to keep charging during the test",
)

// screenRecord enables the screen recorder for the Recorder.
var screenRecord = testing.RegisterVarString(
	"cujrecorder.record",
	"",
	"A boolean string (true/false) signifying whether to record the screen during the test",
)

// ignoreFrameDataError decides whether to ignoring the errors caused by collecting Frame Data.
var ignoreFrameDataError = testing.RegisterVarString(
	"cujrecorder.ignoreFrameDataError",
	"false",
	"A boolean string (true/false) signifying whether to skipping the frame data collecting error",
)

// runPowertop controls whether to run `powertop` during tests.
var runPowertop = testing.RegisterVarString(
	"cujrecorder.runPowertop",
	"",
	"A boolean string (true/false) signifying whether to run powertop for the Recorder",
)

// isLocalVar is a runtime variable that specifies whether to skip
// waiting for the CPU to cooldown and idle, which speeds up overall
// test runtime. This variable is explicitly made for local testing,
// when performance data does not matter.
var isLocalVar = testing.RegisterVarString(
	"cujrecorder.isLocal",
	"",
	"A boolean string (true/false) signifying whether or not to skip certain startup procedures for local testing",
)

// extraChromeCategoriesForTracing is a runtime variable that specifies what
// extra chrome categories will be added to perfetto config for tracing.
// Refer to https://source.chromium.org/chromium/chromium/src/+/refs/heads/main:base/trace_event/builtin_categories.h;l=26-378;drc=b18d59d36ac77ddf968b6e3452109e67471ee38f;bpv=0;bpt=1
// for the list of available categories.
var extraChromeCategoriesForTracing = testing.RegisterVarString(
	"cujrecorder.extraChromeCategoriesForTracing",
	"",
	`A comma-separated string specifying what extra chrome categories should be
	 added to perfetto config for tracing`,
)

// powerLogVar is a runtime variable the specifies whether or not we should
// save power_log.json and power_log.html for each run.
var powerLogVar = testing.RegisterVarString(
	"cujrecorder.powerLog",
	"",
	"A boolean string (true/false) signifying whether to save the power_log.json or power_log.html for each run",
)
