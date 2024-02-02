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

// MotionmarkWPRRecordFixture is a WPR-based fixture for recording Motionmark benchmark source.
const MotionmarkWPRRecordFixture = "loggedInToCUJUserWPRMotionmarkRecord"

// MotionmarkWPRReplayFixture is a WPR-based fixture for running Motionmark benchmark.
const MotionmarkWPRReplayFixture = "loggedInToCUJUserWPRMotionmarkReplay"

// KrakenWPRRecordFixture is a WPR-based fixture for recording Kraken benchmark source.
const KrakenWPRRecordFixture = "loggedInToCUJUserWPRKrakenRecord"

// KrakenWPRReplayFixture is a WPR-based fixture for running Kraken benchmark.
const KrakenWPRReplayFixture = "loggedInToCUJUserWPRKrakenReplay"

// OctaneWPRRecordFixture is a WPR-based fixture for recording Octane benchmark source.
const OctaneWPRRecordFixture = "loggedInToCUJUserWPROctaneRecord"

// OctaneWPRReplayFixture is a WPR-based fixture for running Octane benchmark.
const OctaneWPRReplayFixture = "loggedInToCUJUserWPROctaneReplay"

const speedometerArchive = "ui.BenchmarkCUJ.speedometer.wprgo"
const motionmarkArchive = "ui.BenchmarkCUJ.motionmark.wprgo"
const krakenArchive = "ui.BenchmarkCUJ.kraken.wprgo"
const octaneArchive = "ui.BenchmarkCUJ.octane.wprgo"

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
	testing.AddFixture(cuj.NewWPRLoggedInToCUJUserWithoutCooldownFixture(
		MotionmarkWPRRecordFixture,
		"WPR Record fixture for Motionmark",
		[]string{
			"cros-sw-perf@google.com",
			"skardach@google.com",
		},
		browser.TypeAsh,
		wpr.Record,
		motionmarkArchive))
	testing.AddFixture(cuj.NewWPRLoggedInToCUJUserWithoutCooldownFixture(
		MotionmarkWPRReplayFixture,
		"WPR Replay fixture for Motionmark",
		[]string{
			"cros-sw-perf@google.com",
			"skardach@google.com",
		},
		browser.TypeAsh,
		wpr.Replay,
		motionmarkArchive))
	testing.AddFixture(cuj.NewWPRLoggedInToCUJUserWithoutCooldownFixture(
		KrakenWPRRecordFixture,
		"WPR Record fixture for Kraken",
		[]string{
			"cros-sw-perf@google.com",
			"skardach@google.com",
		},
		browser.TypeAsh,
		wpr.Record,
		krakenArchive))
	testing.AddFixture(cuj.NewWPRLoggedInToCUJUserWithoutCooldownFixture(
		KrakenWPRReplayFixture,
		"WPR Replay fixture for Kraken",
		[]string{
			"cros-sw-perf@google.com",
			"skardach@google.com",
		},
		browser.TypeAsh,
		wpr.Replay,
		krakenArchive))
	testing.AddFixture(cuj.NewWPRLoggedInToCUJUserWithoutCooldownFixture(
		OctaneWPRRecordFixture,
		"WPR Record fixture for Octane",
		[]string{
			"cros-sw-perf@google.com",
			"skardach@google.com",
		},
		browser.TypeAsh,
		wpr.Record,
		octaneArchive))
	testing.AddFixture(cuj.NewWPRLoggedInToCUJUserWithoutCooldownFixture(
		OctaneWPRReplayFixture,
		"WPR Replay fixture for Octane",
		[]string{
			"cros-sw-perf@google.com",
			"skardach@google.com",
		},
		browser.TypeAsh,
		wpr.Replay,
		octaneArchive))
}
