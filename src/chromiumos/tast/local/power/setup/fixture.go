// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
		Name: "powerNoUINoWiFi",
		Desc: "Set up test environment for tests with no UI, no backlight, no WiFi",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerNoUIFixture(PowerTestOptions{
			Wifi:               DisableWifiInterfaces,
			UI:                 DisableUI,
			Backlight:          SetBacklightToZero,
			KeyboardBrightness: SetKbBrightnessToZero,
		}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerNoUIWiFi",
		Desc: "Set up test environment for tests with no UI, no backlight, WiFi set to default",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerNoUIFixture(PowerTestOptions{
			UI:                 DisableUI,
			Backlight:          SetBacklightToZero,
			KeyboardBrightness: SetKbBrightnessToZero,
		}),
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
		Parent:          "powerNoUIWiFi",
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerAshKbbl",
		Desc: "Keyboard backlight default level, recommended for simulating user behavior",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(browser.TypeAsh, PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightness,
		}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerAsh",
		Desc: "Keyboard backlight off, recommended for testing feature power",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(browser.TypeAsh, PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerLacrosKbbl",
		Desc: "Keyboard backlight default level, recommended for simulating user behavior",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(browser.TypeLacros, PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightness,
		}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerLacros",
		Desc: "Keyboard backlight off, recommended for testing feature power",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(browser.TypeLacros, PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}),
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
	powerTestOptions *PowerTestOptions
	cleanup          func(context.Context) error
}

// NewPowerNoUIFixture returns a FixtureImpl to set device to various power test
// options.
func NewPowerNoUIFixture(pto PowerTestOptions) testing.FixtureImpl {
	return &powerNoUIFixture{powerTestOptions: &pto}
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

	su.Add(PowerTest(ctx, nil, *f.powerTestOptions, NewBatteryDischargeFromMode(dischargeMode)))
	if err := su.Check(ctx); err != nil {
		s.Fatal("Power test setup failed: ", err)
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
	recorder *power.Recorder
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
	recorder, err := power.NewRecorder(ctx, 1*time.Second, s.OutDir(), s.TestName())
	if err != nil {
		s.Fatal("Cannot create a new Recorder to collect power metrics: ", err)
	}
	if err := recorder.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := recorder.Start(s.TestContext()); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
	f.recorder = recorder
}

func (f *powerMetricsNoUIFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.recorder.Finish(s.TestContext()); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}

type powerUIFixture struct {
	bt               browser.Type
	powerTestOptions *PowerTestOptions
	cr               *chrome.Chrome
	cleanup          func(context.Context) error
}

// PowerUIFixtureData is return back to tests.
type PowerUIFixtureData struct {
	Bt browser.Type
	Cr *chrome.Chrome
}

// NewPowerUIFixture returns a FixtureImpl to set device to use the specified
// browser and various power test options.
func NewPowerUIFixture(bt browser.Type, pto PowerTestOptions) testing.FixtureImpl {
	return &powerUIFixture{bt: bt, powerTestOptions: &pto}
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

	su.Add(PowerTest(ctx, tconn, *f.powerTestOptions, NewBatteryDischargeFromMode(dischargeMode)))
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
