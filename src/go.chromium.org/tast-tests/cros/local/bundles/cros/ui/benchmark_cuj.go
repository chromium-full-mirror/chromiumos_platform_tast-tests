// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/benchmarkcuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
				ExtraAttr: []string{"group:cuj", "group:crosbolt", "crosbolt_perbuild", "crosbolt_fsi_check"},
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
				Name:      "webxprt4",
				ExtraAttr: []string{"group:cuj", "cuj_weekly"},
				Timeout:   30*time.Minute + cujrecorder.CooldownTimeout,
				Fixture:   "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.WebXPRT4Info,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:      "lacros_webxprt4",
				ExtraAttr: []string{"group:cuj", "cuj_weekly"},
				Timeout:   30*time.Minute + cujrecorder.CooldownTimeout,
				Fixture:   "loggedInToCUJUserLacrosWithoutCooldown",
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
			// Experimental variants.
			{
				Name:    "speedometer_blt_50mb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad50MBWithoutCooldown",
			},
			{
				Name:    "speedometer_blt_1gb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad1GBWithoutCooldown",
			},
			{
				Name:    "speedometer_blt_2gb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad2GBWithoutCooldown",
			},
			{
				Name:    "speedometer_blt_3gb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad3GBWithoutCooldown",
			},
			{
				Name:    "speedometer_blt_4gb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad4GBWithoutCooldown",
			},
			{
				Name:    "motionmark_blt_50mb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad50MBWithoutCooldown",
			},
			{
				Name:    "motionmark_blt_1gb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad1GBWithoutCooldown",
			},
			{
				Name:    "motionmark_blt_2gb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad2GBWithoutCooldown",
			},
			{
				Name:    "motionmark_blt_3gb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad3GBWithoutCooldown",
			},
			{
				Name:    "motionmark_blt_4gb",
				Timeout: defaultTimeout,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_weekly"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad4GBWithoutCooldown",
			},
			{
				Name:    "bmark",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.BmarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				Name:    "lacros_bmark",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserLacrosWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.BmarkInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			// WPR variants.
			{
				Name:      "speedometer_wpr",
				Timeout:   defaultTimeout,
				Fixture:   benchmarkcuj.SpeedometerWPRReplayFixture,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
			{
				// This is a helper test for recording the WPR archive for speedometer_wpr.
				// Resulting archive will be located in /tmp/
				Name:    "speedometer_wpr_record",
				Timeout: defaultTimeout,
				Fixture: benchmarkcuj.SpeedometerWPRRecordFixture,
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
					RecorderMode:  cujrecorder.Benchmark,
				},
			},
		},
	})
}

func BenchmarkCUJ(ctx context.Context, s *testing.State) {
	benchmarkcuj.Run(ctx, s)
}
