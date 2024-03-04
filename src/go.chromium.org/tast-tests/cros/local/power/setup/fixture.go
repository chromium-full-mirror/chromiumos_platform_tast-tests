// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Timeouts for testing.Fixture.
const (
	SetUpTimeout    = 1 * time.Minute
	ResetTimeout    = 1 * time.Minute
	TearDownTimeout = 1 * time.Minute
	PreTestTimeout  = 1 * time.Minute
	PostTestTimeout = 1 * time.Minute
)

// List of power fixtures.
const (
	PowerNoUINoWiFi        = "powerNoUINoWiFi"
	PowerNoUIWiFi          = "powerNoUIWiFi"
	PowerMetricsNoUI       = "powerMetricsNoUI"
	PowerNoUIPlatformAudio = "powerNoUIPlatformAudio"

	// UI
	PowerAsh       = "powerAsh"
	PowerLacros    = "powerLacros"
	PowerAshARC    = "powerAshARC"
	PowerLacrosARC = "powerLacrosARC"

	// Keyboard backlight
	PowerAshKbbl    = "powerAshKbbl"
	PowerLacrosKbbl = "powerLacrosKbbl"

	// With GAIA login
	PowerAshGAIA    = "powerAshGAIA"
	PowerLacrosGAIA = "powerLacrosGAIA"

	// With ramfs setup
	PowerAshRamfs    = "powerAshRamfs"
	PowerLacrosRamfs = "powerLacrosRamfs"

	// With Dark theme
	PowerAshDark    = "powerAshDark"
	PowerLacrosDark = "powerLacrosDark"

	// With Nightlight
	PowerAshNightlight    = "powerAshNightlight"
	PowerLacrosNightlight = "powerLacrosNightlight"

	PowerAshPlatformAudio = "powerAshPlatformAudio"

	// Without Charge Limit
	PowerAshAdaptiveCharging = "powerAshAdaptiveCharging"
	PowerNoChargeLimit       = "powerNoChargeLimit"

	// Launcher Image search
	PowerImageSearchWithFlagOn = "powerImageSearchWithFlagOn"
)

// PowerFixtureOptions describes options used by the fixture only.
type PowerFixtureOptions struct {
	BrowserType      browser.Type
	BrowserExtraOpts []chrome.Option
	EnableGAIALogin  bool
	EnableARC        bool
}

// Register variables for overriding PowerFixtureOptions.

// Extra Chrome features to enable when running any test based on
// powerUIFixture. Note that this will override any feature enable options
// coming from the fixture caller so it is up to the user to ensure that it is
// safe and meaningful to enable given features in scope of a given test.
// This variable is for manual A/B testing scenarios where one would like to
// run a test based on the powerUIFixture without modifying it but ensure some
// features are enabled.
var extraFeaturesVar = testing.RegisterVarString(
	"setup.extraFeatures",
	"",
	"A comma separated list of extra features to be passed into Chrome",
)

// Chrome features to disable when running any test based on powerUIFixture.
// Note that this will override any feature disable options coming from the
// fixture caller so it is up to the user to ensure that it is safe and
// meaningful to disable given features in scope of a given test.
// This variable is for manual A/B testing scenarios where one would like to
// run a test based on the powerUIFixture without modifying it but ensure some
// features are disabled.
var disabledFeaturesVar = testing.RegisterVarString(
	"setup.disabledFeatures",
	"",
	"A comma separated list of disabled features to be passed into Chrome",
)

// Register variables for overriding PowerTestOptions.

// When true, powerd will be kept running during any test based on the
// powerUIFixture irrespective of what the test itself may configure.
// This variable is meant to be used in conjunction with setup.extraFeatures
// and setup.disabledFeatures when a given feature requires powerd to work (e.g.
// to initiate some DBUS requests).
//
// Note: keep in mind that powerd performs runtime tuning of various power
// parameters such as screen brightness (using ambient light sensor if present)
// keyboard backlight, etc. This can make test results dependent on the physical
// setup environment. Be sure that this behavior is intended in your test when
// using this variable.
// b/310050824 tracks creation of a "test mode" which would disable runtime
// tuning in powerd while leaving other features enabled.
var keepPowerdVar = testing.RegisterVarString(
	"setup.keep_powerd",
	"false",
	"keep_powerd decides whether to keep powerd running during the test",
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
		SetUpTimeout:    SetUpTimeout,
		TearDownTimeout: TearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerNoUINoWiFi,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerNoUIWiFi,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerNoUIPlatformAudio,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerMetricsNoUI,
		Desc: "Set up test environment for tests needing no UI or backlight, collect power metrics, visualize and upload data",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl:            &powerMetricsNoUIFixture{},
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout + power.RecorderTimeout,
		PostTestTimeout: PostTestTimeout,
		Parent:          "powerNoUIWiFi",
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAshKbbl,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAsh,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerLacrosKbbl,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerLacros,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAshGAIA,
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
		SetUpTimeout:    chrome.GAIALoginTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerLacrosGAIA,
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
		SetUpTimeout:    chrome.GAIALoginTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAshARC,
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
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerLacrosARC,
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
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAshRamfs,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerLacrosRamfs,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})
	// Dark theme basic fixtures
	testing.AddFixture(&testing.Fixture{
		Name: PowerAshDark,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: PowerLacrosDark,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})
	// Nightlight basic fixtures
	testing.AddFixture(&testing.Fixture{
		Name: PowerAshNightlight,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: PowerLacrosNightlight,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAshPlatformAudio,
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
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
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
				chrome.EnableFeatures("FeatureManagementVideoConference"),
			},
		}),
		Parent:          fixture.AloopLoaded{Channels: 2}.Instance(),
		SetUpTimeout:    chrome.GAIALoginTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAshAdaptiveCharging,
		Desc: "Fixture with Charge Limit disabled",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"dbasehore@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			ChargeLimit: DisableChargeLimit,
			Powerd:      DoNotChangePowerd,
		}, PowerFixtureOptions{
			BrowserType: browser.TypeAsh,
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("AdaptiveCharging"),
			},
		}),
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerNoChargeLimit,
		Desc: "Fixture with Charge Limit disabled",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"dbasehore@google.com",
		},
		Impl: NewPowerNoUIFixture(PowerTestOptions{
			ChargeLimit: DisableChargeLimit,
			Powerd:      DoNotChangePowerd,
		}),
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerImageSearchWithFlagOn,
		Desc: "Fixture with image search flags turned on",
		Contacts: []string{
			"launcher-search-notify@google.com",
			"dgrebenyuk@google.com",
			"xiuwen@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{BrowserType: browser.TypeAsh,
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("ProductivityLauncherImageSearch"),
				chrome.EnableFeatures("LauncherImageSearch"),
				chrome.EnableFeatures("LauncherImageSearchOcr"),
				chrome.EnableFeatures("LauncherImageSearchIca"),
			}}),
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
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
	logRecorder      *power.LogRecorder
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
	f.logRecorder = power.NewLogRecorder(s.OutDir())

	if err := f.logRecorder.Start(); err != nil {
		s.Error("Failed to start log recorder: ", err)
	}
}

func (f *powerNoUIFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.logRecorder.Stop(); err != nil {
		s.Error("Failed to stop log recorder: ", err)
	}

	f.logRecorder = nil
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
	logRecorder *power.LogRecorder
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

	// Apply PowerFixtureOptions feature command-line overrides.
	// The extraFeaturesVar and disabledFeaturesVar are meant for manual testing
	// where user wants to compare results of an unmodified test against a
	// particular feature being enabled and disabled but does not want to create
	// a new test variant or fixture.
	spaceRemover := func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}
	extraFeatures := extraFeaturesVar.Value()
	if extraFeatures != "" {
		extraFeatures = strings.Map(spaceRemover, extraFeatures)
		testing.ContextLog(ctx, "Enabling features: ", extraFeatures)
		opts = append(opts, chrome.EnableFeatures(strings.Split(extraFeatures, ",")...))
	}
	disabledFeatures := disabledFeaturesVar.Value()
	if disabledFeatures != "" {
		disabledFeatures = strings.Map(spaceRemover, disabledFeatures)
		testing.ContextLog(ctx, "Disabling features: ", disabledFeatures)
		opts = append(opts, chrome.DisableFeatures(strings.Split(disabledFeatures, ",")...))
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
		a, err = arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		arcSnapshot, err := arc.NewSnapshot(ctx, a)
		if err != nil {
			s.Fatal("Failed to take ARC state snapshot: ", err)
		}
		f.arcSnapshot = arcSnapshot
	}

	// Apply PowerTestOptions command-line overrides.
	powerTestOptions := f.powerTestOptions
	keepPowerd := keepPowerdVar.Value()
	if keepPowerd == "true" {
		powerTestOptions.Powerd = DoNotChangePowerd
	}

	// Set up the testing environment.
	cleanup, err := PowerTestSetup(ctx, "powerUIFixture", tconn, powerTestOptions)
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
	f.logRecorder = power.NewLogRecorder(s.OutDir())

	if err := f.logRecorder.Start(); err != nil {
		s.Error("Failed to start log recorder: ", err)
	}
}

func (f *powerUIFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.logRecorder.Stop(); err != nil {
		s.Error("Failed to stop log recorder: ", err)
	}
	f.logRecorder = nil

	if f.arc != nil {
		if err := f.arc.SaveLogFiles(ctx); err != nil {
			s.Log("Failed to save ARC-related log files: ", err)
		} else {
			s.Log("ARC-related log files saved successfully")
		}
	}
}
