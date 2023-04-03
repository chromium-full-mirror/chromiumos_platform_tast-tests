// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/cpu"
	"chromiumos/tast/local/power"
	"chromiumos/tast/testing"
)

const (
	setUpTimeout    = 1 * time.Minute
	resetTimeout    = 1 * time.Minute
	tearDownTimeout = 1 * time.Minute
	preTestTimeout  = 1 * time.Minute
	postTestTimeout = 1 * time.Minute
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
		Name: "powerNoUI",
		Desc: "Set up test environment for tests needing no UI or backlight",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl:            &powerNoUIFixture{},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerMetricsNoUI",
		Desc: "Set up test environment for tests needing no UI or backlight, collect power metrics, visualize and upload data",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl:            &powerMetricsNoUIFixture{},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          "powerNoUI",
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerAsh",
		Desc: "Set up test environment for power qual",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl:            &powerUIFixture{bt: browser.TypeAsh},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerLacros",
		Desc: "Set up test environment for power qual",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl:            &powerUIFixture{bt: browser.TypeLacros},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
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

type powerNoUIFixture struct {
	cleanup func(context.Context) error
}

func (f *powerNoUIFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Set up the testing environment.
	su, cleanup := New("powerNoUIFixture")

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

func (f *powerNoUIFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cleanup(ctx); err != nil {
		s.Error("Power cleanup failed: ", err)
	}
}

func (f *powerNoUIFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *powerNoUIFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *powerNoUIFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

type powerMetricsNoUIFixture struct {
	metrics *perf.Timeline
}

func (f *powerMetricsNoUIFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return nil
}

func (f *powerMetricsNoUIFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *powerMetricsNoUIFixture) Reset(ctx context.Context) error {
	return nil
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

	if err := metrics.Start(s.TestContext()); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}

	if err := metrics.StartRecording(s.TestContext()); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	f.metrics = metrics
}

func (f *powerMetricsNoUIFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	p, err := f.metrics.StopRecording(s.TestContext())
	if err != nil {
		s.Fatal("Error while recording power metrics: ", err)
	}

	if err := power.GeneratePowerLogAndSaveToCrosbolt(ctx, s.OutDir(), s.TestName(), p); err != nil {
		s.Error("Failed to generate power_log.json and/or save perf data for crosbolt: ", err)
	}
}

type powerUIFixture struct {
	bt      browser.Type
	cr      *chrome.Chrome
	cleanup func(context.Context) error
}

// PowerUIFixtureData is return back to tests.
type PowerUIFixtureData struct {
	Bt browser.Type
	Cr *chrome.Chrome
}

func (f *powerUIFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Prepare Chrome browser.
	opts := []chrome.Option{
		// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
		// so that each test run does not remember info from last test run.
		// TODO(b/264508768): Add gaia accounts for testing.
		chrome.ExtraArgs("--disable-sync"),
		// b/228256145 to avoid powerd restart.
		chrome.DisableFeatures("FirmwareUpdaterApp"),
	}
	cr, err := browserfixt.NewChrome(ctx, f.bt, lacrosfixt.NewConfig(), opts...)
	if err != nil {
		s.Fatal("Failed to login session: ", err)
	}
	defer func() {
		if s.HasError() {
			cr.Close(cleanupCtx)
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	// Set up the testing environment.
	su, cleanup := New("powerUIFixture")
	defer func() {
		if s.HasError() {
			cleanup(cleanupCtx)
		}
	}()

	dischargeMode := NoBatteryDischarge
	if _, err := power.SysfsBatteryPath(ctx); err == nil {
		dischargeMode = ForceBatteryDischarge
	} else if !errors.Is(err, power.ErrNoBattery) {
		// If it's ErrNoBattery, leave dischargeMode at NoBatteryDischarge.
		s.Log("Unable to determine if a battery exists, do not force discharge: ", err)
	}

	su.Add(PowerTest(ctx, tconn,
		PowerTestOptions{
			Wifi:       DisableWifiInterfaces,
			NightLight: DisableNightLight,
			DarkTheme:  EnableLightTheme,
		},
		NewBatteryDischargeFromMode(dischargeMode),
	))
	if err := su.Check(ctx); err != nil {
		s.Fatal("Power test setup failed: ", err)
	}

	chrome.Lock()
	f.cr = cr
	f.cleanup = cleanup

	return PowerUIFixtureData{Bt: f.bt, Cr: f.cr}
}

func (f *powerUIFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cleanup(ctx); err != nil {
		s.Error("Power cleanup failed: ", err)
	}

	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Error("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}

func (f *powerUIFixture) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	return nil
}

func (f *powerUIFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *powerUIFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
