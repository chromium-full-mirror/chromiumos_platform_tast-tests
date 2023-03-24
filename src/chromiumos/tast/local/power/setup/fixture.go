// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/cpu"
	"chromiumos/tast/local/power"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "powerSetUp",
		Desc: "Set up DUT for power measurements",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"jakebarnes@google.com",
		},
		Impl:            &powerSetUpFixture{},
		SetUpTimeout:    time.Minute,
		TearDownTimeout: time.Minute,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerMetricsNoUI",
		Desc: "Set up test environment, collect power metrics, visualization and upload data",
		Contacts: []string{
			"chromeos-power@google.com",
			"mqg@chromium.org",
		},
		Impl:            &powerMetricsNoUIFixture{},
		SetUpTimeout:    1 * time.Minute,
		ResetTimeout:    1 * time.Minute,
		TearDownTimeout: 1 * time.Minute,
		PreTestTimeout:  1 * time.Minute,
		PostTestTimeout: 1 * time.Minute,
	})
}

type powerSetUpFixture struct {
	cleanup func(context.Context) error
}

func (f *powerSetUpFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	sup, cleanup := New("powerSetUpFixture")

	// Stop UI in order to minimize the number of factors that could influence the results.
	sup.Add(DisableService(ctx, "ui"))

	sup.Add(PowerTest(ctx, nil, PowerTestOptions{
		Wifi: DisableWifiInterfaces,
		// Since we stop the UI disabling the Night Light is redundant.
		NightLight: DoNotDisableNightLight,
	}, nil))

	if err := sup.Check(ctx); err != nil {
		s.Fatal("Power setup failed: ", err)
	}

	f.cleanup = cleanup

	return nil
}

func (f *powerSetUpFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cleanup(ctx); err != nil {
		testing.ContextLog(ctx, "Power cleanup failed: ", err)
	}
}

func (f *powerSetUpFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *powerSetUpFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *powerSetUpFixture) Reset(ctx context.Context) error {
	return nil
}

type powerMetricsNoUIFixture struct {
	cleanup func(context.Context) error
	metrics *perf.Timeline
}

func (f *powerMetricsNoUIFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Set up the testing environment.
	su, cleanup := New("powerMetricsNoUIFixture")

	dischargeMode := NoBatteryDischarge
	if _, err := power.SysfsBatteryPath(ctx); err == nil {
		dischargeMode = ForceBatteryDischarge
	} else if !errors.Is(err, power.ErrNoBattery) {
		// If it's ErrNoBattery, leave dischargeMode at NoBatteryDischarge.
		s.Log("Unable to determine if a battery exists, do not force discharge: ", err)
	}

	su.Add(PowerTest(ctx, nil,
		PowerTestOptions{
			Wifi:               DisableWifiInterfaces,
			UI:                 DisableUI,
			Backlight:          SetBacklightToZero,
			KeyboardBrightness: SetKbBrightnessToZero,
		},
		NewBatteryDischargeFromMode(dischargeMode),
	))
	if err := su.Check(ctx); err != nil {
		s.Error("Power test setup failed: ", err)
	}

	f.cleanup = cleanup

	return nil
}

func (f *powerMetricsNoUIFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cleanup(ctx); err != nil {
		s.Error("Power cleanup failed: ", err)
	}
}

func (f *powerMetricsNoUIFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Wait until CPU is cooled down and idle.
	if _, err := cpu.WaitUntilCoolDown(ctx, cpu.IdleCoolDownConfig()); err != nil {
		s.Error("CPU failed to cool down: ", err)
	}
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Error("CPU failed to idle: ", err)
	}

	metrics, err := perf.NewTimeline(ctx, power.TestMetrics(), perf.Interval(1*time.Second))
	if err != nil {
		s.Fatal("Failed to build metrics: ", err)
	}

	f.metrics = metrics

	if err := metrics.Start(s.TestContext()); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}

	if err := metrics.StartRecording(s.TestContext()); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}
}

func (f *powerMetricsNoUIFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	p, err := f.metrics.StopRecording(s.TestContext())
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

func (f *powerMetricsNoUIFixture) Reset(ctx context.Context) error {
	return nil
}
