// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/ui/benchmarkcuj"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/ui/cujrecorder"

	"go.chromium.org/tast/core/testing"
)

const defaultTimeout = 15*time.Minute + cujrecorder.CooldownTimeout

func init() {
	testing.AddTest(&testing.Test{
		Func:         BenchmarkCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "CUJ running browser benchmarks",
		Contacts: []string{
			"chromeos-perfmetrics-eng@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome", "no_android_vm_t"},
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
				Timeout: 25 * time.Minute,
				Fixture: "loggedInToCUJUser",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.JetStreamInfo,
				},
			},
			{
				Name:    "lacros_jetstream",
				Timeout: 25 * time.Minute,
				Fixture: "loggedInToCUJUserLacros",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.JetStreamInfo,
				},
				ExtraSoftwareDeps: []string{"lacros"},
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
				Name:    "lacros_kraken",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserLacros",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.KrakenInfo,
				},
				ExtraSoftwareDeps: []string{"lacros"},
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
			{
				Name:    "lacros_octane",
				Timeout: defaultTimeout,
				Fixture: "loggedInToCUJUserLacros",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.OctaneInfo,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

func BenchmarkCUJ(ctx context.Context, s *testing.State) {
	benchmarkcuj.Run(ctx, s)
}
