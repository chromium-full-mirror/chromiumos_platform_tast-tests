// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/benchmarkcuj"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/testing"
)

const defaultTimeout = 15*time.Minute + cujrecorder.CooldownTimeout

func init() {
	testing.AddTest(&testing.Test{
		Func:         BenchmarkCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "CUJ running browser benchmarks",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			// Iterations are limited by the specific benchmark:
			// Speedometer: Default to 10.
			"iterations",
		},
		Params: []testing.Param{
			{
				Name:      "speedometer",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:      "lacros_speedometer",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserLacrosWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "motionmark",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:      "lacros_motionmark",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserLacrosWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "jetstream",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.JetStreamInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:      "lacros_jetstream",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserLacrosWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.JetStreamInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "kraken",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.KrakenInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:      "lacros_kraken",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserLacrosWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.KrakenInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "octane",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.OctaneInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:      "lacros_octane",
				ExtraAttr: []string{"group:cuj"},
				Timeout:   defaultTimeout,
				Fixture:   "loggedInToCUJUserLacrosWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.OctaneInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "webxprt4",
				Timeout: 30*time.Minute + cujrecorder.CooldownTimeout,
				Fixture: "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.WebXPRT4Info,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:    "lacros_webxprt4",
				Timeout: 30*time.Minute + cujrecorder.CooldownTimeout,
				Fixture: "loggedInToCUJUserLacrosWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.WebXPRT4Info,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			// Battery saver tests only run manually
			{
				Name:    "battery_saver_speedometer",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:    "battery_saver_motionmark",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:    "battery_saver_jetstream",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.JetStreamInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:    "battery_saver_kraken",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.KrakenInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:    "battery_saver_octane",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.OctaneInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:    "battery_saver_webxprt4",
				Timeout: 30*time.Minute + cujrecorder.CooldownTimeout,
				Fixture: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.WebXPRT4Info,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
		},
	})
}

func BenchmarkCUJ(ctx context.Context, s *testing.State) {
	benchmarkcuj.Run(ctx, s)
}
