// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/cpu"
	"chromiumos/tast/local/power"
	"chromiumos/tast/local/power/setup"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IdleNoUI,
		Desc:         "Collect power metrics when device is in idle with no UI",
		BugComponent: "b:167191",
		Contacts:     []string{"chromeos-power@google.com"},
		// Disabled because this is an example test for other tests to follow.
		// Attr:      []string{"group:mainline", "informational"},
		Timeout: 3 * time.Minute,
	})
}

func IdleNoUI(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up the testing environment.
	su, cleanup := setup.New("power.IdleNoUI")
	defer cleanup(cleanupCtx)

	dischargeMode := setup.NoBatteryDischarge
	if _, err := power.SysfsBatteryPath(ctx); err == nil {
		dischargeMode = setup.ForceBatteryDischarge
	} else if !errors.Is(err, power.ErrNoBattery) {
		// If it's ErrNoBattery, leave dischargeMode at NoBatteryDischarge.
		s.Log("Unable to determine if a battery exists, do not force discharge: ", err)
	}

	su.Add(setup.PowerTest(ctx, nil,
		setup.PowerTestOptions{
			Wifi:               setup.DisableWifiInterfaces,
			UI:                 setup.DisableUI,
			Backlight:          setup.SetBacklightToZero,
			KeyboardBrightness: setup.SetKbBrightnessToZero,
		},
		setup.NewBatteryDischargeFromMode(dischargeMode),
	))
	if err := su.Check(ctx); err != nil {
		s.Error(err, "Power test setup failed: ", err)
	}

	// Wait until CPU is cooled down and idle.
	_, err := cpu.WaitUntilCoolDown(ctx, cpu.IdleCoolDownConfig())
	if err != nil {
		s.Error("CPU failed to cool down: ", err)
	}
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Error("CPU failed to idle: ", err)
	}

	metrics, err := perf.NewTimeline(ctx, power.TestMetrics(), perf.Interval(1*time.Second))
	if err != nil {
		s.Fatal("Failed to build metrics: ", err)
	}

	if err := metrics.Start(ctx); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}

	if err := metrics.StartRecording(ctx); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	// Start of main test body. Idle for 10 seconds while reading power metrics
	// every second. This both serves as an example for future power tests and
	// as a light weight test to test the device setup. Replace this chunk of
	// code with functionality code for future power tests.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// End of main test body.

	p, err := metrics.StopRecording(ctx)
	if err != nil {
		s.Fatal("Error while recording power metrics: ", err)
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}

	if err := power.GeneratePowerLogAndSaveToCrosbolt(ctx, s.OutDir(), s.TestName(), p); err != nil {
		s.Error("Failed to generate power_log.json and/or save perf data for crosbolt: ", err)
	}
}
