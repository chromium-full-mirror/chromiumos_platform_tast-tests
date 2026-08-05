// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/facegaze"
	"go.chromium.org/tast-tests/cros/local/a11y/mousekeys"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testType int

const (
	testTypeARC testType = iota
	testTypeBrowser
	testTypeFaceGaze
	testTypeFocusMode
	testTypeMouseKeys
)

const (
	idleDuration   = 10 * time.Minute
	emptyWindowURL = "about:blank"
)

type idlePerfTest struct {
	testType testType
}

func init() {
	testing.AddTest(&testing.Test{
		Func: IdlePerf,
		Desc: "Measures the CPU usage while the desktop is idle",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			cujrecorder.SystemTraceConfigFile,
			facegaze.FakeCameraVideoFile720p,
		},
		Timeout: 12*time.Minute + cuj.CPUStabilizationTimeout + idleDuration,
		Params: []testing.Param{{
			Val:               idlePerfTest{testType: testTypeARC},
			Fixture:           "arcBootedRestricted",
			ExtraSoftwareDeps: []string{"arc"},
		}, {
			Name: "arc_disabled",
			Val: idlePerfTest{
				testType: testTypeBrowser,
			},
			Fixture: "chromeLoggedInDisableSync",
		}, {
			Name: "facegaze",
			Val: idlePerfTest{
				testType: testTypeFaceGaze,
			},
			Fixture: fixture.ChromeLoggedInDisableSyncWithFaceGaze,
		}, {
			// TODO(b/356944093): Remove focusmode fixture from `idle_perf` once feature launches.
			Name: "focusmode",
			Val: idlePerfTest{
				testType: testTypeFocusMode,
			},
			ExtraAttr: []string{"cuj_weekly"},
			Fixture:   fixture.ChromeLoggedInWithFocusMode,
		}},
	})
}

func IdlePerf(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

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
		fallthrough
	case testTypeFaceGaze:
		fallthrough
	case testTypeFocusMode:
		fallthrough
	case testTypeMouseKeys:
		cr = s.FixtValue().(chrome.HasChrome).Chrome()
	}

	// Wait for cpu to stabilize before test. This should be done here
	// instead of as a recorder option, since the FaceGaze experiment will not
	// cooldown once the setup has completed.
	if _, err := cpu.WaitUntilStabilized(ctx, cujrecorder.CPUCoolDownConfig()); err != nil {
		s.Log("Failed to wait for CPU to become idle: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get Test API connection: ", err)
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Recorder with no additional config; it records and reports memory usage and
	// CPU percents of browser/GPU processes.
	recorder, err := cujrecorder.NewRecorder(ctx, tconn, cr, a, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer func() {
		if err := recorder.Close(closeCtx); err != nil {
			s.Error("Failed to stop recorder: ", err)
		}
	}()

	switch idleTest.testType {
	case testTypeBrowser:
		conn, err := cr.NewConn(ctx, emptyWindowURL)
		if err != nil {
			s.Fatalf("Failed to open %s: %v", emptyWindowURL, err)
		}
		defer conn.Close()
	case testTypeFaceGaze:
		metric := []cujrecorder.MetricConfig{
			cujrecorder.NewCustomMetricConfig("Accessibility.FaceGaze.AverageFaceLandmarkerLatency", "ms", perf.SmallerIsBetter),
		}

		if err := recorder.AddCollectedMetrics(metric...); err != nil {
			s.Fatal("Failed to add FaceGaze metric to the recorder: ", err)
		}

		facegazeDriver, err := facegaze.SetUp(ctx, cr, s.DataPath)
		if err != nil {
			s.Fatal("Failed to set up FaceGaze: ", err)
		}

		defer func() {
			if err := facegazeDriver.TearDown(); err != nil {
				s.Error("Failed to tear down FaceGaze: ", err)
			}
		}()
	case testTypeFocusMode:
		if err := quicksettings.EnsureFocusModeHasStarted(ctx, tconn); err != nil {
			s.Fatal("Failed to start a Focus session: ", err)
		}

		defer func() {
			if err := quicksettings.EnsureFocusModeEnds(closeCtx, tconn); err != nil {
				s.Error("Failed to end Focus Mode: ", err)
			}
		}()
	case testTypeMouseKeys:
		kb, err := input.KeyboardWithCustomDelay(ctx, a11y.MouseKeysDefaultDelay+time.Second)
		if err != nil {
			s.Fatal("Failed to create keyboard: ", err)
		}

		if err := mousekeys.SetUp(ctx, kb, cr); err != nil {
			s.Error("Failed to setup MouseKeys: ", err)
		}

		// Perform a series of mouse movements and clicks.
		if err := mousekeys.PerformActionsForIdleTest(ctx, kb); err != nil {
			s.Error("Failed to perform mouse actions: ", err)
		}

		defer func() {
			if err := mousekeys.TearDown(closeCtx, kb, cr, tconn); err != nil {
				s.Error("Failed to tear down MouseKeys: ", err)
			}
		}()
	}
	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

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
