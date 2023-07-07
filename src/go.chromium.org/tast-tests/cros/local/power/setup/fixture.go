// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/power"
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

// PowerFixtureOptions describes options used by the fixture only.
type PowerFixtureOptions struct {
	BrowserType      browser.Type
	BrowserExtraOpts []chrome.Option
	EnableGAIALogin  bool
	EnableARC        bool
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "powerSetUp",
		Desc: "Set up DUT for power measurements",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"jakebarnes@google.com",
		},
		Impl:            &powerSetUpFixture{},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
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
		Name: "powerNoUIPlatformAudio",
		Desc: "The powerNoUINoWiFi fixture customized for testing platform audio features that do not depend on ash running",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
			"aaronyu@google.com",
		},
		Impl: NewPowerNoUIFixture(
			PowerTestOptions{
				UI: DisableUI,
				// Audio should be handled within the test itself.
				Audio: DoNotChangeAudio,
				// Minimize interference.
				KeyboardBrightness: SetKbBrightnessToZero,
				Wifi:               DisableWifiInterfaces,
				Backlight:          SetBacklightToZero,
			},
		),
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
		PreTestTimeout:  preTestTimeout + power.RecorderTimeout,
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
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightness,
		}, PowerFixtureOptions{BrowserType: browser.TypeAsh}),
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
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{BrowserType: browser.TypeAsh}),
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
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightness,
		}, PowerFixtureOptions{BrowserType: browser.TypeLacros}),
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
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{BrowserType: browser.TypeLacros}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerAshGAIA",
		Desc: "Keyboard backlight off with GAIA login, recommended for testing feature power",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserType:     browser.TypeAsh,
			EnableGAIALogin: true,
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerLacrosGAIA",
		Desc: "Keyboard backlight off with GAIA login, recommended for testing feature power",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserType:     browser.TypeLacros,
			EnableGAIALogin: true,
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerAshARC",
		Desc: "Keyboard backlight off with ARC enabled, recommended for testing feature power",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserType:     browser.TypeAsh,
			EnableGAIALogin: true,
			EnableARC:       true,
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerLacrosARC",
		Desc: "Lacros variation of powerAshARC",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserType:     browser.TypeLacros,
			EnableGAIALogin: true,
			EnableARC:       true,
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerAshRamfs",
		Desc: "PowerAsh with ramfs setup for local data",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
			Ramfs:              SetupRamfs,
		}, PowerFixtureOptions{BrowserType: browser.TypeAsh}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerLacrosRamfs",
		Desc: "PowerLacros with ramfs setup for local data",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
			Ramfs:              SetupRamfs,
		}, PowerFixtureOptions{BrowserType: browser.TypeLacros}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	// Dark theme basic fixtures
	testing.AddFixture(&testing.Fixture{
		Name: "powerAshDark",
		Desc: "Dark theme version of powerAsh",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableDarkTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{BrowserType: browser.TypeAsh}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "powerLacrosDark",
		Desc: "Dark theme version of powerLacros",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableDarkTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{BrowserType: browser.TypeLacros}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	// Nightlight basic fixtures
	testing.AddFixture(&testing.Fixture{
		Name: "powerAshNightlight",
		Desc: "Nightlight version of powerAsh",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         EnableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{BrowserType: browser.TypeAsh}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "powerLacrosNightlight",
		Desc: "Nightlight version of powerLacros",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         EnableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{BrowserType: browser.TypeLacros}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerAshPlatformAudio",
		Desc: "PowerAsh customized for testing platform audio features",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
			"aaronyu@google.com",
		},
		Impl: NewPowerUIFixture(
			PowerTestOptions{
				// CRAS depends on Chrome for DLC and features service.
				UI: DoNotChangeUI,
				// Audio should be handled within the test itself.
				Audio: DoNotChangeAudio,
				// Minimize interference.
				KeyboardBrightness: SetKbBrightnessToZero,
				Wifi:               DisableWifiInterfaces,
				Backlight:          SetBacklightToZero,
				// Disable these features even though we already set brightness to 0,
				// just in case that nightlight/dark theme brings stress to the CPU.
				NightLight: DisableNightLight,
				DarkTheme:  EnableLightTheme,
			},
			PowerFixtureOptions{
				BrowserType: browser.TypeAsh,
				BrowserExtraOpts: []chrome.Option{
					// Prevent interference of audio preferences.
					// See go/tast-fakecrasaudioclient.
					chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
					// Feature flags.
					chrome.EnableFeatures("CrOSLateBootAudioAPNoiseCancellation"),
				},
			},
		),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "powerAshGAIAWithSpeakOnMute",
		Desc: "Fixture for speak-on-mute power test",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
			"aaronyu@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
			Wifi:               DisableWifiInterfaces,
			Audio:              DoNotChangeAudio, // Uses audio.
		}, PowerFixtureOptions{
			BrowserType:     browser.TypeAsh,
			EnableGAIALogin: true,
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("CrosPrivacyHub"),
				chrome.EnableFeatures("VideoConference"),
			},
		}),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.GAIALoginTimeout + setUpTimeout,
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
	}, NewBatteryDischarge(false /*discharge*/, true /*ignoreErr*/, DefaultDischargeThreshold)))

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
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Set up the testing environment.
	cleanup, err := PowerTestSetup(ctx, "powerNoUIFixture", nil, f.powerTestOptions)
	if err != nil {
		s.Fatal("Power fixture failed: ", err)
	}
	defer func() {
		if s.HasError() {
			cleanup(cleanupCtx)
		}
	}()

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
	recorder := power.NewRecorder(ctx, 1*time.Second, s.OutDir(), s.TestName())
	if err := recorder.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := recorder.Start(s.TestContext()); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
	f.recorder = recorder
}

func (f *powerMetricsNoUIFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	defer f.recorder.Close(ctx)

	if err := f.recorder.Finish(s.TestContext()); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}

// gaiaLoginOption fetches the power test accounts and password then combine them with credential format.
// Test accounts reuse accounts from autotest.
func gaiaLoginOption(ctx context.Context) (chrome.Option, error) {
	const (
		pltpBaseURL = "https://sites.google.com/a/chromium.org/dev/chromium-os/testing/power-testing/pltp"
		pltuURL     = pltpBaseURL + "/pltu_rand"
		pltpURL     = pltpBaseURL + "/pltp_rand"
	)

	usernames, err := utils.FetchFromURL(ctx, pltuURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch usernames")
	}
	password, err := utils.FetchFromURL(ctx, pltpURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch password")
	}

	names := strings.Split(usernames, "\n")
	password = strings.TrimSuffix(password, "\n")
	var loginPool string
	// loginPool is a string containing multiple credentials separated by newlines:
	//
	// user1:pass1
	// user2:pass2
	// user3:pass3
	for _, n := range names {
		loginPool += fmt.Sprintf("%s:%s", n, password) + "\n"
	}

	return chrome.GAIALoginPool(loginPool), nil
}

type powerUIFixture struct {
	powerTestOptions   *PowerTestOptions
	powerFixtureOption *PowerFixtureOptions

	cr          *chrome.Chrome
	arc         *arc.ARC
	arcSnapshot *arc.Snapshot
	cleanup     func(context.Context) error
}

// PowerUIFixtureData is return back to tests.
type PowerUIFixtureData struct {
	Bt  browser.Type
	Cr  *chrome.Chrome
	ARC *arc.ARC
}

// NewPowerUIFixture returns a FixtureImpl to set device to use the specified
// browser, various power test options and power fixture options.
func NewPowerUIFixture(pto PowerTestOptions, pfo PowerFixtureOptions) testing.FixtureImpl {
	return &powerUIFixture{powerTestOptions: &pto, powerFixtureOption: &pfo}
}

func (f *powerUIFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Prepare Chrome browser.
	opts := []chrome.Option{
		// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
		// so that each test run does not remember info from last test run.
		chrome.ExtraArgs("--disable-sync"),
		// Prefer using constant frame rate for camera streaming.
		chrome.ExtraArgs("--enable-features=PreferConstantFrameRate"),
		// Allow 2 windows side by side.
		chrome.ExtraArgs("--force-tablet-mode=clamshell"),
		// b/228256145 to avoid powerd restart.
		chrome.DisableFeatures("FirmwareUpdaterApp"),
	}
	opts = append(opts, f.powerFixtureOption.BrowserExtraOpts...)

	if f.powerFixtureOption.EnableGAIALogin {
		gaiaLoginOpt, err := gaiaLoginOption(ctx)
		if err != nil {
			s.Fatal("Failed to get GAIA login chrome option: ", err)
		}
		opts = append(opts, gaiaLoginOpt)
	}

	if f.powerFixtureOption.EnableARC {
		opts = append(opts,
			chrome.ARCSupported(),
			chrome.ExtraArgs(arc.DisableSyncFlags()...))
	}

	bt := f.powerFixtureOption.BrowserType
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig(), opts...)
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

	var a *arc.ARC
	if f.powerFixtureOption.EnableARC {
		s.Log("Opting into Play Store")
		if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
			s.Fatal("Failed to optin to Play Store: ", err)
		}
		a, err = arc.New(ctx, s.OutDir())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		arcSnapshot, err := arc.NewSnapshot(ctx, a)
		if err != nil {
			s.Fatal("Failed to take ARC state snapshot: ", err)
		}
		f.arcSnapshot = arcSnapshot
	}

	// Set up the testing environment.
	cleanup, err := PowerTestSetup(ctx, "powerUIFixture", tconn, f.powerTestOptions)
	if err != nil {
		s.Fatal("Power fixture failed: ", err)
	}
	defer func() {
		if s.HasError() {
			cleanup(cleanupCtx)
		}
	}()

	chrome.Lock()
	f.cr = cr
	f.arc = a
	f.cleanup = cleanup

	return PowerUIFixtureData{Bt: bt, Cr: f.cr, ARC: f.arc}
}

func (f *powerUIFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cleanup(ctx); err != nil {
		s.Error("Power cleanup failed: ", err)
	}

	chrome.Unlock()

	if f.arc != nil {
		if err := f.arc.Close(ctx); err != nil {
			s.Log("Failed to close ARC: ", err)
		}
	}
	f.arc = nil

	if err := f.cr.Close(ctx); err != nil {
		s.Error("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}

func (f *powerUIFixture) Reset(ctx context.Context) error {
	if f.arc != nil {
		return f.arcSnapshot.Restore(ctx, f.arc)
	}
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
	if f.arc != nil {
		if err := f.arc.SaveLogFiles(ctx); err != nil {
			s.Log("Failed to save ARC-related log files: ", err)
		} else {
			s.Log("ARC-related log files saved successfully")
		}
	}
}
