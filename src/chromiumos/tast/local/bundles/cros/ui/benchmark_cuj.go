// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/ui/benchmarkcuj"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/testing"
)

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
		SoftwareDeps: []string{"chrome"},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				Name:    "speedometer",
				Fixture: "loggedInToCUJUser",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
				},
			},
			{
				Name:    "lacros_speedometer",
				Fixture: "loggedInToCUJUserLacros",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.SpeedometerInfo,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "motionmark",
				Fixture: "loggedInToCUJUser",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
				},
			},
			{
				Name:    "lacros_motionmark",
				Fixture: "loggedInToCUJUserLacros",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.MotionMarkInfo,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "jetstream",
				Fixture: "loggedInToCUJUser",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeAsh,
					BenchmarkInfo: benchmarkcuj.JetStreamInfo,
				},
			},
			{
				Name:    "lacros_jetstream",
				Fixture: "loggedInToCUJUserLacros",
				Val: benchmarkcuj.BenchmarkTest{
					BrowserType:   browser.TypeLacros,
					BenchmarkInfo: benchmarkcuj.JetStreamInfo,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

func BenchmarkCUJ(ctx context.Context, s *testing.State) {
	benchmarkcuj.Run(ctx, s)
}
