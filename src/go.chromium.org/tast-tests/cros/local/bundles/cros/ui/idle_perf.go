// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testType int

const (
	testTypeARC testType = iota
	testTypeBrowser
)

const (
	idleDuration   = 10 * time.Minute
	emptyWindowURL = "about:blank"
)

type idlePerfTest struct {
	testType    testType
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         IdlePerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the CPU usage while the desktop is idle",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@chromium.org",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      cuj.CPUStablizationTimeout + idleDuration,

		Params: []testing.Param{{
			Val:     idlePerfTest{testType: testTypeARC},
			Fixture: "arcBootedRestricted",
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: idlePerfTest{
				testType:    testTypeBrowser,
				browserType: browser.TypeLacros,
			},
			Fixture: "lacrosDisableSync",
		}, {
			Name: "ash",
			Val: idlePerfTest{
				testType:    testTypeBrowser,
				browserType: browser.TypeAsh,
			},
			Fixture: "chromeLoggedInDisableSync",
		}},
	})
}

func IdlePerf(ctx context.Context, s *testing.State) {
	idleTest := s.Param().(idlePerfTest)

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	var cr *chrome.Chrome
	var a *arc.ARC
	switch idleTest.testType {
	case testTypeARC:
		cr = s.FixtValue().(*arc.PreData).Chrome
		a = s.FixtValue().(*arc.PreData).ARC
	case testTypeBrowser:
		cr = s.FixtValue().(chrome.HasChrome).Chrome()
	}

	// Wait for cpu to stabilize before test.
	if _, err := cpu.WaitUntilStabilized(ctx, cuj.CPUCoolDownConfig()); err != nil {
		// Log the cpu stabilizing wait failure instead of make it fatal.
		// TODO(b/213238698): Include the error as part of test data.
		s.Log("Failed to wait for CPU to become idle: ", err)
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	var bTconn *chrome.TestConn
	if idleTest.testType == testTypeBrowser {
		conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, idleTest.browserType, emptyWindowURL)
		if err != nil {
			s.Fatalf("Failed to open %s: %v", emptyWindowURL, err)
		}
		bTconn, err = br.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to get browser test API connection: ", err)
		}
		defer closeBrowser(closeCtx)
		defer conn.Close()
	}

	// Recorder with no additional config; it records and reports memory usage and
	// CPU percents of browser/GPU processes.
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, a, cujrecorder.RecorderOptions{Mode: cujrecorder.Perf})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer func() {
		if err := recorder.Close(closeCtx); err != nil {
			s.Error("Failed to stop recorder: ", err)
		}
	}()

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		s.Log("Just wait for ", idleDuration, " to check the load of idle status")
		// GoBigSleepLint sleep to check the load for the device's idle state.
		return testing.Sleep(ctx, idleDuration)
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
