// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/inputlatency"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KeyboardPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test ARC keyboard system performance",
		Contacts: []string{
			"arc-performance@google.com",
			"hungmn@chromium.org",
			"alanding@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Data:         inputlatency.AndroidData(),
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

func KeyboardPerf(ctx context.Context, s *testing.State) {
	// Disable multicast to make sure CPU can be stabilized for capturing performance metrics
	cleanup, err := setup.DisableMulticastSetup(ctx)
	if err != nil {
		s.Fatal("Could not disable multicast: ", err)
	}
	defer cleanup(ctx)

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Could not open Test API connection: ", err)
	}

	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	s.Log("Creating virtual keyboard")
	kbd, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Unable to create virtual keyboard: ", err)
	}
	defer kbd.Close(ctx)

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

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatalf("Unable to launch %s/%s: %v", appName, activityName, err)
	}
	defer act.Stop(ctx, tconn)

	if err := inputlatency.WaitForCPUStabilized(ctx); err != nil {
		s.Fatal("Failed to wait until CPU is stabilized: ", err)
	}

	if err := inputlatency.WaitForClearUI(ctx, d, nil); err != nil {
		s.Fatal("Failed to clear UI: ", err)
	}

	s.Log("Injecting key events")
	const (
		numEvents = 50
		waitMS    = 50
	)
	eventTimes := make([]int64, 0, numEvents)
	for i := 0; i < numEvents; i += 2 {
		if err := inputlatency.WaitForNextEventTime(ctx, a, &eventTimes, waitMS); err != nil {
			s.Fatal("Failed to generate event time: ", err)
		}
		if err := kbd.AccelPress(ctx, "a"); err != nil {
			s.Fatal("Unable to inject key events: ", err)
		}

		if err := inputlatency.WaitForNextEventTime(ctx, a, &eventTimes, waitMS); err != nil {
			s.Fatal("Failed to generate event time: ", err)
		}
		if err := kbd.AccelRelease(ctx, "a"); err != nil {
			s.Fatal("Unable to inject key events: ", err)
		}
	}

	pv := perf.NewValues()
	if err := inputlatency.EvaluateLatency(ctx, s, d, numEvents, eventTimes, "avgKeyboardLatency", nil, pv); err != nil {
		s.Fatal("Failed to evaluate: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}
