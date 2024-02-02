// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmarkcuj

import (
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/wpr"
	"go.chromium.org/tast/core/testing"
)

// SpeedometerWPRRecordFixture is a WPR-based fixture for recording Speedometer2.1 benchmark source.
const SpeedometerWPRRecordFixture = "loggedInToCUJUserWPRSpeedometerRecord"

// SpeedometerWPRReplayFixture is a WPR-based fixture for running Speedometer2.1 benchmark.
const SpeedometerWPRReplayFixture = "loggedInToCUJUserWPRSpeedometerReplay"

const speedometerArchive = "ui.BenchmarkCUJ.speedometer.wprgo"

func init() {
	// As a workaround for lack of parametrized fixtures (see b/285970864) add
	// parametrized fixtures to not pollute the cros/chrome/cuj/fixture.go.
	// Also note that the lint issue here is due to the fact that it expects an
	// explicit structure definition, not a function call.
	testing.AddFixture(cuj.NewWPRLoggedInToCUJUserWithoutCooldownFixture(
		SpeedometerWPRRecordFixture,
		"WPR Record fixture for Speedometer 2.1",
		[]string{
			"cros-sw-perf@google.com",
			"skardach@google.com",
		},
		browser.TypeAsh,
		wpr.Record,
		speedometerArchive))
	testing.AddFixture(cuj.NewWPRLoggedInToCUJUserWithoutCooldownFixture(
		SpeedometerWPRReplayFixture,
		"WPR Replay fixture for Speedometer 2.1",
		[]string{
			"cros-sw-perf@google.com",
			"skardach@google.com",
		},
		browser.TypeAsh,
		wpr.Replay,
		speedometerArchive))
}
