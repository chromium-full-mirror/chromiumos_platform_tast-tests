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

	"go.chromium.org/tast-tests/cros/common/fixture"
	cf "go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	powerUtil "go.chromium.org/tast-tests/cros/local/power/util"
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
	PowerOobe              = "powerOobe"

	// UI
	PowerAsh                     = "powerAsh"
	PowerAshARC                  = "powerAshARC"
	PowerAshARCWithTuwunelServer = "powerAshARCWithTuwunelServer"

	// Keyboard backlight
	PowerAshKbbl = "powerAshKbbl"

	// With GAIA login
	PowerAshGAIA = "powerAshGAIA"

	// With ramfs setup
	PowerAshRamfs    = "powerAshRamfs"
	PowerAshARCRamfs = "powerAshARCRamfs"

	// With Dark theme
	PowerAshDark = "powerAshDark"

	// With Nightlight
	PowerAshNightlight = "powerAshNightlight"

	// For platform audio test scheme
	PowerAshPlatformAudio                  = "powerAshPlatformAudio"
	PowerAshPlatformAudioNoiseCancellation = "powerAshPlatformAudioNoiseCancellation"
	PowerAshPlatformAudioStyleTransfer     = "powerAshPlatformAudioStyleTransfer"

	// Without Charge Limit
	PowerAshAdaptiveCharging = "powerAshAdaptiveCharging"
	PowerNoChargeLimit       = "powerNoChargeLimit"

	// Launcher Image search
	PowerImageSearchWithFlagOn = "powerImageSearchWithFlagOn"

	// Read Aloud
	PowerAshReadAloudWithFlagOn = "powerAshReadAloudWithFlagOn"

	// Protected video playback
	PowerAshProtectedVideo = "powerAshProtectedVideo"

	// Mahi
	PowerAshMahi            = "powerAshMahi"
	PowerAshMahiAndSimplify = "powerAshMahiAndSimplify"

	// Regmon
	PowerAshRegmonEnabled  = "powerAshRegmonEnabled"
	PowerAshRegmonDisabled = "powerAshRegmonDisabled"

	// Captions on Braille Display
	PowerAshCaptionsOnBraille = "powerAshCaptionsOnBraille"
)

// PowerFixtureOptions describes options used by the fixture only.
type PowerFixtureOptions struct {
	BrowserExtraOpts []chrome.Option
	ExtraOptsFunc    chrome.OptionsCallback
	EnableGAIALogin  bool
	EnableARC        bool
	EnableHDR        bool
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

// When true, the PowerTestOptions will be overridden with
// powerTestOptions.Audio = DoNotChangeAudio.
// Usage: tast run -var=setup.keep_audio=true <dut> <tests>
var keepAudioVar = testing.RegisterVarString(
	"setup.keep_audio",
	"false",
	"keep_audio decides whether to keep audio unchanged when setting up the test",
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         PowerNoUINoWiFi,
		Desc:         "Set up test environment for tests with no UI, no backlight, no WiFi",
		BugComponent: "b:1361410",
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
		Name:         PowerNoUIWiFi,
		Desc:         "Set up test environment for tests with no UI, no backlight, WiFi set to default",
		BugComponent: "b:1361410",
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
		Name:         PowerOobe,
		Desc:         "Set up test environment for measuring OOBE provisioning time",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"zactu@chromium.org",
		},
		Impl: NewPowerNoUIFixture(PowerTestOptions{
			UI:           RestartUI,
			ChargeLimit:  DisableChargeLimit,
			Powerd:       DoNotChangePowerd,
			UpdateEngine: DoNotChangeUpdateEngine,
			VNC:          DoNotChangeVNC,
			Avahi:        DoNotChangeAvahi,
			DPTF:         DoNotChangeDPTF,
			Audio:        DoNotChangeAudio,
			Bluetooth:    DoNotChangeBluetooth,
			Multicast:    DoNotChangeMulticast,
		}),
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerNoUIPlatformAudio,
		Desc:         "The powerNoUINoWiFi fixture customized for testing platform audio features that do not depend on ash running",
		BugComponent: "b:1361410",
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
		Name:         PowerMetricsNoUI,
		Desc:         "Set up test environment for tests needing no UI or backlight, collect power metrics, visualize and upload data",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl:            &powerMetricsNoUIFixture{},
		SetUpTimeout:    SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout + cp.RecorderCooldownTimeout,
		PostTestTimeout: PostTestTimeout + cp.RecorderOverheadTimeout,
		Parent:          "powerNoUIWiFi",
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshKbbl,
		Desc:         "Keyboard backlight default level, recommended for simulating user behavior",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightness,
		}, PowerFixtureOptions{}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAsh,
		Desc:         "Keyboard backlight off, recommended for testing feature power",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "powerAshLowRefresh",
		Desc:         "Force low refresh rate",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"ddavenport@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				// Force refresh rate throttling to be active
				chrome.ExtraArgs("--force-refresh-rate-throttle"),
				// Feature flags.
				chrome.EnableFeatures("SeamlessRefreshRateSwitching"),
			},
		}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "powerAshHighRefresh",
		Desc:         "Force high refresh rate",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"ddavenport@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				// Feature flags.
				chrome.DisableFeatures("SeamlessRefreshRateSwitching"),
			},
		}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "powerAshVRR",
		Desc:         "Force VRR to be enabled",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"ddavenport@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				// Feature flags.
				chrome.DisableFeatures("SeamlessRefreshRateSwitching"),
				chrome.EnableFeatures("EnableVariableRefreshRateAlwaysOn"),
			},
		}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshGAIA,
		Desc:         "Keyboard backlight off with GAIA login, recommended for testing feature power",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			EnableGAIALogin: true,
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshARC,
		Desc:         "Keyboard backlight off with ARC enabled, recommended for testing feature power",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			EnableGAIALogin: true,
			EnableARC:       true,
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshARCWithTuwunelServer,
		Desc:         "Keyboard backlight off with ARC enabled and Tuwunel server setup, recommended for testing feature power",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			EnableGAIALogin: true,
			EnableARC:       true,
		}),
		Parent:          fixture.TuwunelServerSetup,
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshRamfs,
		Desc:         "PowerAsh with ramfs setup for local data",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
			Ramfs:              SetupRamfs,
		}, PowerFixtureOptions{EnableHDR: true}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshARCRamfs,
		Desc:         "PowerAsh with ramfs setup for local data with ARC enabled",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"jingmuli@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
			Ramfs:              SetupRamfs,
		}, PowerFixtureOptions{
			EnableGAIALogin: true,
			EnableARC:       true,
			EnableHDR:       true}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	// Dark theme basic fixtures
	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshDark,
		Desc:         "Dark theme version of powerAsh",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableDarkTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	// Nightlight basic fixtures
	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshNightlight,
		Desc:         "Nightlight version of powerAsh",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         EnableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshPlatformAudioNoiseCancellation,
		Desc:         "PowerAsh customized for testing platform audio noise cancellation",
		BugComponent: "b:1361410",
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
				BrowserExtraOpts: []chrome.Option{
					// Prevent interference of audio preferences.
					// See go/tast-fakecrasaudioclient.
					chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
					// Feature flags.
					chrome.EnableFeatures("CrOSLateBootAudioAPNoiseCancellation"),
					chrome.DisableFeatures("CrOSLateBootAudioStyleTransfer"),
				},
			},
		),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshPlatformAudioStyleTransfer,
		Desc:         "PowerAsh customized for testing platform audio style transfer",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
			"cranelw@google.com",
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
				BrowserExtraOpts: []chrome.Option{
					// Prevent interference of audio preferences.
					// See go/tast-fakecrasaudioclient.
					chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
					// Feature flags.
					chrome.EnableFeatures("CrOSLateBootAudioAPNoiseCancellation", "CrOSLateBootAudioStyleTransfer"),
				},
			},
		),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshPlatformAudio,
		Desc:         "PowerAsh customized for testing platform audio",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
			"johnylin@google.com",
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
				BrowserExtraOpts: []chrome.Option{
					// Prevent interference of audio preferences.
					// See go/tast-fakecrasaudioclient.
					chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
				},
			},
		),
		Parent:          "rebootForAudioDSPFixture",
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshAdaptiveCharging,
		Desc:         "Fixture with Charge Limit disabled",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			ChargeLimit: DisableChargeLimit,
			Powerd:      DoNotChangePowerd,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("AdaptiveCharging"),
			},
		}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerNoChargeLimit,
		Desc:         "Fixture with Charge Limit disabled",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
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
			"chenjih@google.com",
			"xiuwen@google.com",
		},
		// ChromeOS > Software > Consumer > Machine Intelligence > Search
		BugComponent: "b:1257106",
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("LauncherImageSearchIca"),
			}}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAshReadAloudWithFlagOn,
		Desc: "Fixture with read aloud flags turned on ash",
		Contacts: []string{
			"komo-eng@google.com",
			"trewin@google.com",
		},
		BugComponent: "b:1372781",
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("ReadAnythingWebUIToolbar"),
				chrome.EnableFeatures("ReadAnythingReadAloud"),
			}}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshProtectedVideo,
		Desc:         "Like PowerAsh but allows protected video",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"jkardatzke@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				// Options for allowing L1 playback in dev mode
				chrome.ExtraArgs("--allow-ra-in-dev-mode"),
				chrome.ExtraArgs("--unsafely-allow-protected-media-identifier-for-domain=127.0.0.1"),
				// Feature flags.
				chrome.EnableFeatures("EnableArmHwdrm"),
			}}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshMahi,
		Desc:         "Fixture with mahi flags enabled and feature key provided",
		BugComponent: "b:1551116",
		Contacts: []string{
			"alanlxl@google.com",
			"chenjih@google.com",
			"thanhdng@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("Mahi"),
			},
			ExtraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return []chrome.Option{
					chrome.ExtraArgs("--mahi-feature-key=" + s.RequiredVar("mahi.featureTestKey")),
					chrome.ExtraArgs("--mahi-restrictions-override"),
				}, nil
			},
		}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		Vars:            []string{"mahi.featureTestKey"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshMahiAndSimplify,
		Desc:         "Fixture with mahi flag (for simplify feature) enabled",
		BugComponent: "b:1673015",
		Contacts: []string{
			"alanlxl@google.com",
			"thanhdng@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("Mahi"),
			},
			ExtraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return []chrome.Option{
					chrome.ExtraArgs("--mahi-restrictions-override"),
				}, nil
			},
		}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshRegmonEnabled,
		Desc:         "Fixture with Regmon feature enabled",
		BugComponent: "b:1129862",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				// Fake login, to also support setting policies.
				chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
				// Enable Regmon feature flags.
				chrome.ExtraArgs("--enable-features=CrOSLateBootRegmonPolicyMonitoringEnabled,NetworkAnnotationMonitoring"),
			},
			ExtraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				fdms, ok := s.ParentValue().(*fakedms.FakeDMS)
				if !ok {
					s.Fatal("Parent is not a FakeDMS fixture")
				}
				return []chrome.Option{
					// FakeDMS for setting policies.
					chrome.DMSPolicy(fdms.URL),
				}, nil
			},
		}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		Parent:          cf.FakeDMS, // Allow setting policies
	})

	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshRegmonDisabled,
		Desc:         "Fixture with Regmon feature disabled",
		BugComponent: "b:1129862",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				// Fake login, to also support setting policies.
				chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
				// Enable Regmon feature flags.
				chrome.ExtraArgs("--disable-features=CrOSLateBootRegmonPolicyMonitoringEnabled,NetworkAnnotationMonitoring"),
			},
			ExtraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				fdms, ok := s.ParentValue().(*fakedms.FakeDMS)
				if !ok {
					s.Fatal("Parent is not a FakeDMS fixture")
				}
				return []chrome.Option{
					// FakeDMS for setting policies.
					chrome.DMSPolicy(fdms.URL),
				}, nil
			},
		}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		Parent:          cf.FakeDMS, // Allow setting policies
	})

	testing.AddFixture(&testing.Fixture{
		Name: PowerAshCaptionsOnBraille,
		Desc: "Fixture with captions on braille feature enabled",
		Contacts: []string{
			"chromeos-a11y-eng@google.com",
			"xiyuan@google.com",
		},
		BugComponent: "b:1272895",
		Impl: NewPowerUIFixture(PowerTestOptions{
			NightLight:         DisableNightLight,
			DarkTheme:          EnableLightTheme,
			KeyboardBrightness: SetKbBrightnessToZero,
		}, PowerFixtureOptions{
			BrowserExtraOpts: []chrome.Option{
				chrome.EnableFeatures("CaptionsOnBrailleDisplay"),
			}}),
		SetUpTimeout:    minCapacityChargeTimeout + SetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: TearDownTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
	})
}

type powerNoUIFixture struct {
	powerTestOptions *PowerTestOptions
	logRecorder      *power.LogRecorder
	cleanup          func(context.Context) error
}

// PowerNoUIFixtureData is return back to tests.
type PowerNoUIFixtureData struct {
	Discharge bool
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
	cleanup, discharge, err := PowerTestSetup(ctx, "powerNoUIFixture", nil, f.powerTestOptions)
	if err != nil {
		s.Fatal("Power fixture failed: ", err)
	}
	defer func() {
		if s.HasError() {
			cleanup(cleanupCtx)
		}
	}()

	f.cleanup = cleanup

	return PowerNoUIFixtureData{Discharge: discharge}
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
	discharge bool
	recorder  *power.Recorder
}

func (f *powerMetricsNoUIFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	f.discharge = s.ParentValue().(PowerNoUIFixtureData).Discharge
	return nil
}

func (f *powerMetricsNoUIFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *powerMetricsNoUIFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *powerMetricsNoUIFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	recorder := power.NewRecorder(ctx, 1*time.Second, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(f.discharge))
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
		pltpBaseURL = "https://storage.googleapis.com/chromiumos-test-assets-public/power_LoadTest/account"
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
		if n == "" {
			continue
		}
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
	fdms        *fakedms.FakeDMS
	logRecorder *power.LogRecorder
	cleanup     func(context.Context) error
}

// PowerUIFixtureData is return back to tests.
type PowerUIFixtureData struct {
	Discharge bool
	Cr        *chrome.Chrome
	ARC       *arc.ARC
	Fdms      *fakedms.FakeDMS
	WithAudio bool
}

// Chrome returns Chrome. This adds support for chrome.HasChrome interface.
func (fd PowerUIFixtureData) Chrome() *chrome.Chrome {
	return fd.Cr
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
		// Hide privacy sandbox dialog.
		chrome.EnableFeatures("PrivacySandboxSettings4"),
		// Prevents media provider from scanning external volumes which causes
		// high CPU usage for a long period of time.
		chrome.DisableFeatures("ArcExternalStorageAccess"),
		// Disable the optimization model downloads which caused extra power consumption
		// on page loading.
		chrome.ExtraArgs("--disable-optimization-guide-model-downloads-for-benchmarking"),
	}
	opts = append(opts, f.powerFixtureOption.BrowserExtraOpts...)
	if keepAudioVar.Value() == "true" {
		// Prevent interference of audio preferences.
		// See go/tast-fakecrasaudioclient.
		opts = append(opts, chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"))
	}

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

	if f.powerFixtureOption.EnableHDR {
		opts = append(opts, chrome.EnableHDR())
	}

	if f.powerFixtureOption.ExtraOptsFunc != nil {
		extraOpts, err := f.powerFixtureOption.ExtraOptsFunc(ctx, s)
		if err != nil {
			s.Fatal("Failed to get extra options: ", err)
		}
		opts = append(opts, extraOpts...)
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

	cr, err := chrome.New(ctx, opts...)
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
	if keepAudioVar.Value() == "true" {
		powerTestOptions.Audio = DoNotChangeAudio
	}
	withAudio := powerTestOptions.Audio == DoNotChangeAudio

	status, err := power.GetStatus(ctx)
	if err != nil {
		s.Fatal("Failed to obtain DUT power status: ", err)
	}
	if status.BatteryPresent && powerUtil.SupportChromeEC() {
		// Ensure the battery is higher than threshold before calling PowerTestSetup.
		lowBatteryShutdownPercent, err := metrics.LowBatteryShutdownPercent(ctx)
		if err != nil {
			s.Fatal("Invalid battery_shutdown_percent: ", err)
		}
		threshold := DefaultDischargeThreshold + lowBatteryShutdownPercent

		chargeParams := power.ChargeParams{
			MinChargePercentage: threshold,
			MaxChargePercentage: 100.0,
		}
		if err := PrepareBattery(ctx, chargeParams); err != nil {
			s.Fatal("Failed to prepare battery: ", err)
		}
	}

	// Set up the testing environment.
	cleanup, discharge, err := PowerTestSetup(ctx, "powerUIFixture", tconn, powerTestOptions)
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

	fdms, _ := s.ParentValue().(*fakedms.FakeDMS)
	f.fdms = fdms

	return PowerUIFixtureData{Discharge: discharge, Cr: f.cr, ARC: f.arc, Fdms: f.fdms, WithAudio: withAudio}
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

	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get ash tconn")
	}

	// Ensures that there are no toplevel windows left open.
	if all, err := ash.GetAllWindows(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to call ash.GetAllWindows")
	} else if len(all) != 0 {
		return errors.Wrapf(err, "toplevel window (%q) stayed open, total %d left", all[0].Name, len(all))
	}

	if f.powerTestOptions.Multicast == DisableMulticast && IsEthernetConnected(ctx) {
		multcastCleanup, err := DisableAllMulticast(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to disable multicast")
		}
		cleanup := f.cleanup
		f.cleanup = func(ctx context.Context) error {
			lastErr := multcastCleanup(ctx)
			if err := cleanup(ctx); err != nil {
				lastErr = err
			}
			return lastErr
		}
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
