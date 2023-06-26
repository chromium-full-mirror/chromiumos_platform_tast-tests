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
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "speedometer",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
				},
			},
			{
				Name:    "lacros_speedometer",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserLacrosWithoutCooldown",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "motionmark",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUser",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
				},
			},
			{
				Name:    "lacros_motionmark",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserLacros",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "jetstream",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUser",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.JetStreamInfo,
				},
			},
			{
				Name:    "kraken",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUser",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.KrakenInfo,
				},
			},
			{
				Name:    "octane",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUser",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.OctaneInfo,
				},
			},
		},
	})
}

func BenchmarkCUJ(ctx context.Context, s *testing.State) {
	benchmarkcuj.Run(ctx, s)
}
