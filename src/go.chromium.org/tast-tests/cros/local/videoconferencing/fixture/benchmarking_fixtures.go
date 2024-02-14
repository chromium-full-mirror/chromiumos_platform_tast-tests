// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// List of fixture names for Video Conferencing tests.
const (
	// GAIALoggedInAndBenchmarkSetupFixture is a fixture which logs in using OTA and sets up a DUT for power measurements.
	GAIALoggedInAndBenchmarkSetupFixture = "gaiaLoggedInAndBenchmarkSetupFixture"

	// LoggedInAndBenchmarkSetupFixture is a fixture which logs in using test user and sets up a DUT for power measurements.
	LoggedInAndBenchmarkSetupFixture = "loggedInAndBenchmarkSetupFixture"

	// PowerLoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder is a fixture used for power measurements with standard power api.
	PowerLoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder = "powerLoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder"
)

var keepWifiVar = testing.RegisterVarString(
	"fixture.keep_wifi",
	"false",
	"keep_wifi decides whether to keep wifi enabled by default",
)
var keepPowerdVar = testing.RegisterVarString(
	"fixture.keep_powerd",
	"false",
	"keep_powerd decides whether to prevent disabling powerd",
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInAndBenchmarkSetupFixture,
		Desc: "Enter a fresh session logged in with OTA, setup power, disable wifi and screen recorder",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"zhaon@google.com",
		},
		Impl:            &benchmarkSetUpFixture{},
		Parent:          GAIALoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder,
		SetUpTimeout:    2 * time.Minute,
		ResetTimeout:    30 * time.Second,
		TearDownTimeout: 30 * time.Second,
	})

	testing.AddFixture(&testing.Fixture{
		Name: LoggedInAndBenchmarkSetupFixture,
		Desc: "Log in with a fake test user, setup power, disable wifi and screen recorder",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"zhaon@google.com",
		},
		Impl:            &benchmarkSetUpFixture{},
		Parent:          LoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder,
		SetUpTimeout:    2 * time.Minute,
		ResetTimeout:    30 * time.Second,
		TearDownTimeout: 30 * time.Second,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerLoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder,
		Desc: "Log in with a fake powerloadtest user, setup power, disable wifi and screen recorder",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"xiuwen@chromium.org",
		},
		Impl: setup.NewPowerUIFixture(setup.PowerTestOptions{
			NightLight:         setup.DisableNightLight,
			DarkTheme:          setup.EnableLightTheme,
			KeyboardBrightness: setup.SetKbBrightnessToZero,
			Wifi:               setup.DisableWifiInterfaces,
		}, setup.PowerFixtureOptions{
			BrowserType:     browser.TypeAsh,
			EnableGAIALogin: true,
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("VCBackgroundReplace"),
			},
		}),
		Parent:          LoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder,
		SetUpTimeout:    chrome.GAIALoginTimeout + setup.SetUpTimeout,
		ResetTimeout:    setup.ResetTimeout,
		TearDownTimeout: setup.TearDownTimeout,
		PreTestTimeout:  setup.PreTestTimeout,
		PostTestTimeout: setup.PostTestTimeout,
	})
}

// BenchmarkSetUpFixtureData is provided to the test to use the Chrome instance.
type BenchmarkSetUpFixtureData struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
}

type benchmarkSetUpFixture struct {
	cr           *chrome.Chrome
	tconn        *chrome.TestConn
	powerCleanup setup.CleanupCallback
}

func (f *benchmarkSetUpFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	var err error
	f.cr = s.ParentValue().(chrome.HasChrome).Chrome()
	f.tconn, err = f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	options := setup.PowerTestOptions{
		NightLight: setup.DisableNightLight,
	}
	keepState := keepWifiVar.Value()
	if keepState == "false" {
		options.Wifi = setup.DisableWifiInterfaces
	}
	keepPowerd := keepPowerdVar.Value()
	if keepPowerd == "true" {
		options.Powerd = setup.DoNotChangePowerd
	}

	cleanup, err := setup.PowerTestSetup(ctx, "powerUIFixture", f.tconn, &options)
	if err != nil {
		s.Fatal("Power fixture failed: ", err)
	}
	defer func() {
		if s.HasError() {
			cleanup(cleanupCtx)
		}
	}()
	f.powerCleanup = cleanup

	return BenchmarkSetUpFixtureData{Chrome: f.cr, TestAPIConn: f.tconn}
}

func (f *benchmarkSetUpFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.powerCleanup != nil {
		if err := f.powerCleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Power cleanup failed: ", err)
		}
	}
}

func (f *benchmarkSetUpFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *benchmarkSetUpFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *benchmarkSetUpFixture) Reset(ctx context.Context) error {
	return nil
}
