// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/perfetto"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/inputlatency"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MousePerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test ARC mouse system performance",
		Contacts: []string{
			"arc-performance@google.com",
			"hungmn@chromium.org",
			"alanding@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Data:         append(inputlatency.AndroidData(), "perfetto_config.pbtxt"),
		Params: []testing.Param{{
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "arcBootedWithDisableExternalStorage",
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"android_container", "lacros"},
			Fixture:           "lacrosWithArcBootedAndDisableExternalStorage",
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "arcBootedWithDisableExternalStorage",
		}, {
			Name:              "vm_lacros",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Fixture:           "lacrosWithArcBootedAndDisableExternalStorage",
		}},
		Timeout: 10 * time.Minute,
	})
}

func MousePerf(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Disable powerd to ensure the device doesn't go to sleep while waiting for CPU to be stabilized
	cleanup, err := setup.PowerTestSetup(ctx, "powerd disabled", nil, &setup.PowerTestOptions{
		Powerd: setup.DisablePowerd,
	})
	if err != nil {
		s.Fatal("Failed to disable powerd: ", err)
	}
	defer cleanup(cleanupCtx)

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Could not open Test API connection: ", err)
	}

	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	s.Log("Creating virtual mouse")
	m, err := input.Mouse(ctx)
	if err != nil {
		s.Fatal("Unable to create virtual mouse: ", err)
	}
	defer m.Close(ctx)

	if err := inputlatency.InstallArcHostClockClient(ctx, a, s); err != nil {
		s.Fatal("Could not install arc-host-clock-client: ", err)
	}

	const (
		apkName      = "ArcInputLatencyTest.apk"
		appName      = "org.chromium.arc.testapp.inputlatency"
		activityName = ".MainActivity"
	)
	s.Log("Installing " + apkName)
	if err := a.Install(ctx, arc.APKPath(apkName)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	s.Logf("Launching %s/%s", appName, activityName)
	act, err := arc.NewActivity(a, appName, activityName)
	if err != nil {
		s.Fatalf("Unable to create new activity %s/%s: %v", appName, activityName, err)
	}
	defer act.Close(ctx)

	sdkVersion, err := arc.SDKVersion()
	if err != nil {
		s.Fatal("ailed to get SDK version: ", err)
	}
	ctxStartApp, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if sdkVersion >= arc.SDKR {
		traceResultPath := filepath.Join(s.OutDir(), "activity_start.trace")
		if err := perfetto.Trace(ctx, a, s.DataPath("perfetto_config.pbtxt"), traceResultPath, false, func(ctx context.Context) error {
			if err := act.StartWithDefaultOptions(ctxStartApp, tconn); err != nil {
				return errors.Wrapf(err, "Unable to launch %s/%s", appName, activityName)
			}
			return nil
		}); err != nil {
			s.Fatal("Error on run perfetto trace on activity start: ", err)
		}
	} else {
		if err := act.StartWithDefaultOptions(ctxStartApp, tconn); err != nil {
			s.Fatalf("Unable to launch %s/%s: %v", appName, activityName, err)
		}
	}
	defer act.Stop(ctx, tconn)

	if err := act.SetWindowState(ctx, tconn, arc.WindowStateFullscreen); err != nil {
		s.Fatal("Could not maximize test app: ", err)
	}
	if err := ash.WaitForARCAppWindowState(ctx, tconn, appName, ash.WindowStateFullscreen); err != nil {
		s.Fatal("The test app is not in fullscreen: ", err)
	}

	if err := inputlatency.WaitForCPUStabilized(ctx); err != nil {
		s.Fatal("Failed to wait until CPU is stabilized: ", err)
	}

	// Check latency for mouse ACTION_MOVE events which are generated when moving mouse after left-button pressing down and holding.
	s.Log("Injecting mouse press-down move events")
	const (
		numEvents     = 100
		numLeftClicks = 20
		waitMS        = 50
		y             = 0
	)
	eventTimes := make([]int64, 0, numEvents)
	if err := m.Press(); err != nil {
		s.Fatal("Unable to inject Press mouse event: ", err)
	}
	if err := inputlatency.WaitForSomeEvents(ctx, d, nil); err != nil {
		s.Fatal("Failed to wait for events to show up: ", err)
	}
	if err := inputlatency.WaitForClearUI(ctx, d, nil); err != nil {
		s.Fatal("Failed to clear UI: ", err)
	}
	var x int32 = 10
	for i := 0; i < numEvents; i++ {
		if x == 10 {
			x = -10
		} else {
			x = 10
		}
		if err := inputlatency.WaitForNextEventTime(ctx, a, &eventTimes, waitMS); err != nil {
			s.Fatal("Failed to generate event time: ", err)
		}
		if err := m.Move(x, y); err != nil {
			s.Fatal("Unable to inject Move mouse event: ", err)
		}
	}

	pv := perf.NewValues()

	if err := inputlatency.EvaluateLatency(ctx, s, d, numEvents, eventTimes, "avgMouseLeftMoveLatency", nil, pv); err != nil {
		s.Fatal("Failed to evaluate: ", err)
	}

	if err := m.Release(); err != nil {
		s.Fatal("Unable to inject Release mouse event: ", err)
	}
	if err := inputlatency.WaitForClearUI(ctx, d, nil); err != nil {
		s.Fatal("Failed to clear UI: ", err)
	}

	s.Log("Injecting mouse left-click events")
	// When left-clicking on mouse, it injects ACTION_DOWN, ACTION_BUTTON_PRESS, ACTION_UP, and ACTION_BUTTON_RELEASE.
	// The framework also injects three additional actions: ACTION_HOVER_EXIT, ACTION_HOVER_ENTER, and ACTION_HOVER_ENTER for P / ACTION_HOVER_MOVE for R+.
	// Check latency for these actions.
	numLeftClickGroupEvents := 7

	numLeftClickEvents := numLeftClicks * numLeftClickGroupEvents
	eventTimes = make([]int64, 0, numLeftClickEvents)
	for i := 0; i < numLeftClicks; i++ {
		if err := inputlatency.WaitForNextEventTime(ctx, a, &eventTimes, waitMS); err != nil {
			s.Fatal("Failed to generate event time: ", err)
		}
		lastEventTime := eventTimes[len(eventTimes)-1]
		// ACTION_HOVER_EXIT, ACTION_DOWN and ACTION_BUTTON_PRESS are generated for mouse press.
		eventTimes = append(eventTimes, lastEventTime, lastEventTime)
		if err := m.Press(); err != nil {
			s.Fatal("Unable to inject Press mouse event: ", err)
		}
		if err := inputlatency.WaitForNextEventTime(ctx, a, &eventTimes, waitMS); err != nil {
			s.Fatal("Failed to generate event time: ", err)
		}
		lastEventTime = eventTimes[len(eventTimes)-1]
		// ACTION_UP, ACTION_BUTTON_RELEASE, ACTION_HOVER_ENTER, ACTION_HOVER_MOVE/ACTION_HOVER_ENTER for mouse release.
		eventTimes = append(eventTimes, lastEventTime, lastEventTime, lastEventTime)
		if err := m.Release(); err != nil {
			s.Fatal("Unable to inject Release mouse event: ", err)
		}
	}

	if err := inputlatency.EvaluateLatency(ctx, s, d, numLeftClickEvents, eventTimes, "avgMouseLeftClickLatency", nil, pv); err != nil {
		s.Fatal("Failed to evaluate: ", err)
	}

	// Clear data to start next test.
	if err := inputlatency.WaitForClearUI(ctx, d, nil); err != nil {
		s.Fatal("Failed to clear UI: ", err)
	}

	s.Log("Injecting the mouse hover-move events")
	// Additional ACTION_HOVER_ENTER event is generated for the first mouse hover-move event.
	if err := m.Move(x, y); err != nil {
		s.Fatal("Unable to inject mouse hover-move event: ", err)
	}
	if err := inputlatency.WaitForSomeEvents(ctx, d, nil); err != nil {
		s.Fatal("Failed to wait for events to show up: ", err)
	}
	if err := inputlatency.WaitForClearUI(ctx, d, nil); err != nil {
		s.Fatal("Failed to clear UI: ", err)
	}

	eventTimes = make([]int64, 0, numEvents)
	for i := 0; i < numEvents; i++ {
		if x == 10 {
			x = -10
		} else {
			x = 10
		}
		if err := inputlatency.WaitForNextEventTime(ctx, a, &eventTimes, waitMS); err != nil {
			s.Fatal("Failed to generate event time: ", err)
		}
		if err := m.Move(x, y); err != nil {
			s.Fatal("Unable to inject mouse hover-move event: ", err)
		}
	}
	if err := inputlatency.EvaluateLatency(ctx, s, d, numEvents, eventTimes, "avgMouseHoverMoveLatency", nil, pv); err != nil {
		s.Fatal("Failed to evaluate: ", err)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}
