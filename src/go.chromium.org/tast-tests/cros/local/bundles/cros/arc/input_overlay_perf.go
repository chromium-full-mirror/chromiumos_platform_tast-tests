// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/gio"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/inputlatency"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	numEvents     = 50
	numMoveEvents = 100
	// TODO(b/258229512): reduce the wait times after the inputlatency package issues are resolved.
	tapWaitMs  = 250
	moveWaitMs = 250
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputOverlayPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the performance of inputs for input overlay",
		Contacts:     []string{"arc-app-dev@google.com", "arc-performance@google.com", "pjlee@google.com", "cuicuiruan@google.com"},
		// ChromeOS > Software > ARC++ > Gaming
		BugComponent: "b:1373988",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Data:         inputlatency.AndroidData(),
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container"},
			}, {
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			}},
		Timeout: 5 * time.Minute,
	})
}

func InputOverlayPerf(ctx context.Context, s *testing.State) {
	gio.SetupTestApp(ctx, s, func(params gio.TestParams) error {
		// Wait for the CPU to idle before performing the test.
		if _, err := cpu.WaitUntilCoolDown(ctx, inputOverlayCoolDownConfig()); err != nil {
			s.Fatal("Failed to wait until CPU is cooled down: ", err)
		}
		// Start up keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard")
		}
		defer kb.Close(ctx)

		// Install the ARC host clock.
		if err := inputlatency.InstallArcHostClockClient(ctx, params.Arc, s); err != nil {
			return errors.Wrap(err, "could not install arc-host-clock-client")
		}

		// Store page name in variable.
		var pkgName string = "org.chromium.arc.testapp.inputoverlay"

		// Inject the described number of tap events.
		tapEventTimes := make([]int64, 0, numMoveEvents)
		for i := 0; i < numEvents; i += 2 {
			if err := inputlatency.WaitForNextEventTime(ctx, params.Arc, &tapEventTimes, tapWaitMs); err != nil {
				return errors.Wrap(err, "failed to generate event time")
			}
			if err := kb.AccelPress(ctx, "Space"); err != nil {
				return errors.Wrap(err, "unable to inject key events")
			}

			if err := inputlatency.WaitForNextEventTime(ctx, params.Arc, &tapEventTimes, tapWaitMs); err != nil {
				return errors.Wrap(err, "failed to generate event time")
			}
			if err := kb.AccelRelease(ctx, "Space"); err != nil {
				return errors.Wrap(err, "unable to inject key events")
			}
		}

		// Calculate input latency and save metrics.
		pv := perf.NewValues()
		if err := inputlatency.EvaluateLatency(ctx, s, params.Device, numEvents, tapEventTimes, "avgInputOverlayKeyboardTouchTapLatency", &pkgName, pv); err != nil {
			s.Fatal("Failed to evaluate: ", err)
		}

		if err := inputlatency.WaitForClearUI(ctx, params.Device, &pkgName); err != nil {
			s.Fatal("Failed to clear UI: ", err)
		}

		// Inject the described number of move events.
		// For this simulation, we alternate between pressing the "w" key and the "a"
		// key, while keeping at least one key pressed at all times, to continually
		// inject "ACTION_MOVE" events.
		moveEventTimes := make([]int64, 0, numEvents+2)
		recordEventTime := func() action.Action {
			return func(ctx context.Context) error {
				if err := inputlatency.WaitForNextEventTime(ctx, params.Arc, &moveEventTimes, moveWaitMs); err != nil {
					return errors.Wrap(err, "failed to generate event time")
				}
				return nil
			}
		}
		// Press first key and throw away the first actions.
		if err := kb.AccelPressAction(gio.UpMoveKey)(ctx); err != nil {
			return errors.Wrap(err, "failed to press first key")
		}
		if err := action.Sleep(5 * time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		if err := inputlatency.WaitForClearUI(ctx, params.Device, &pkgName); err != nil {
			s.Fatal("Failed to clear UI: ", err)
		}
		// Continue to inject move actions.
		for i := 0; i < numEvents; i += 4 {
			if err := uiauto.Combine("Continually inject move actions",
				// Press "a" key.
				recordEventTime(),
				kb.AccelPressAction(gio.LeftMoveKey),
				// Lift "w" key.
				recordEventTime(),
				kb.AccelReleaseAction(gio.UpMoveKey),
				// Press "w" key.
				recordEventTime(),
				kb.AccelPressAction(gio.UpMoveKey),
				// Lift "a" key.
				recordEventTime(),
				kb.AccelReleaseAction(gio.LeftMoveKey),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to inject move events")
			}
		}

		if err := inputlatency.EvaluateLatency(ctx, s, params.Device, numEvents+2, moveEventTimes, "avgInputOverlayKeyboardTouchMoveLatency", &pkgName, pv); err != nil {
			s.Fatal("Failed to evaluate: ", err)
		}
		if err := pv.Save(s.OutDir()); err != nil {
			return errors.Wrap(err, "failed saving perf data")
		}

		// Release final "w" key.
		if err := kb.AccelReleaseAction(gio.UpMoveKey)(ctx); err != nil {
			return errors.Wrap(err, "failed to release last key")
		}

		return nil
	})
}

// inputOverlayCoolDownConfig returns the config to wait for the machine to cooldown for game performance tests.
// This overrides the default config timeout (5 minutes) and temperature threshold (46 C)
// settings to reduce test flakes on low-end devices.
func inputOverlayCoolDownConfig() cpu.CoolDownConfig {
	cdConfig := cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)
	cdConfig.PollTimeout = 7 * time.Minute
	cdConfig.TemperatureThreshold = 61000
	return cdConfig
}
