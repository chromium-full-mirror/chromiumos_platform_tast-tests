// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

const (
	debugLacrosTestWaitDuration = 2 * time.Minute

	// This URL has black text on a white page. It automatically scrolls up and down.
	textScrollingURL = "https://petermcneeleychromium.github.io/small_scroll_text/index.html"

	// This URL has a background color that switches between orange and blue at 60fps.
	colorChangeURL = "https://petermcneeleychromium.github.io/color_change_60fps/index.html"
)

type debugLacrosTest struct {
	windowURL   string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DebugLacrosPerf,
		LacrosStatus: testing.LacrosVariantExists,
		// TODO(https://crbug.com/1401138): Evaluate the long-term utility
		Desc:         "Temporary tests to help debug lacros performance issues",
		Contacts:     []string{"erikchen@chromium.org", "chromeos-perfmetrics-eng@google.com"},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      cuj.CPUStablizationTimeout + debugLacrosTestWaitDuration,

		Params: []testing.Param{{
			Name: "test_scroll_ash",
			Val: debugLacrosTest{
				browserType: browser.TypeAsh,
				windowURL:   textScrollingURL,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name:              "text_scroll_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: debugLacrosTest{
				browserType: browser.TypeLacros,
				windowURL:   textScrollingURL,
			},
			Fixture: "loggedInToCUJUserLacros",
		}, {
			Name: "color_change_ash",
			Val: debugLacrosTest{
				browserType: browser.TypeAsh,
				windowURL:   colorChangeURL,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name:              "color_change_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: debugLacrosTest{
				browserType: browser.TypeLacros,
				windowURL:   colorChangeURL,
			},
			Fixture: "loggedInToCUJUserLacros",
		}},
	})
}

func DebugLacrosPerf(ctx context.Context, s *testing.State) {
	debugLacrosTest := s.Param().(debugLacrosTest)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, debugLacrosTest.browserType, debugLacrosTest.windowURL)
	if err != nil {
		s.Fatalf("Failed to open %s: %v", debugLacrosTest.windowURL, err)
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get browser test API connection: ", err)
	}
	defer closeBrowser(closeCtx)
	defer conn.Close()

	// Recorder with no additional config; it records and reports memory usage and
	// CPU percents of browser/GPU processes.
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer func() {
		if err := recorder.Close(closeCtx); err != nil {
			s.Error("Failed to stop recorder: ", err)
		}
	}()

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	recorder.EnableTracing(s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile))

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		s.Logf("Wait for %v to gather more data", debugLacrosTestWaitDuration)
		return testing.Sleep(ctx, debugLacrosTestWaitDuration)
	}); err != nil {
		s.Fatal("Failed to run the test scenario: ", err)
	}

	pv := perf.NewValues()
	if err = recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}
	if err = pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
}
