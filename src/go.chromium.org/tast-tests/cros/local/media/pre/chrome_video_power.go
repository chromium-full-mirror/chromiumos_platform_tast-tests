// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"

	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
)

// MinPowerTestOptions provides an unified stable power environment.
var MinPowerTestOptions = powersetup.PowerTestOptions{
	Wifi:               powersetup.DoNotChangeWifiInterfaces,
	NightLight:         powersetup.DisableNightLight,
	DarkTheme:          powersetup.EnableLightTheme,
	UI:                 powersetup.DoNotChangeUI,
	Ramfs:              powersetup.DoNotSetupRamfs,
	Powerd:             powersetup.DisablePowerd,
	UpdateEngine:       powersetup.DisableUpdateEngine,
	VNC:                powersetup.DisableVNC,
	Avahi:              powersetup.DisableAvahi,
	DPTF:               powersetup.DisableDPTF,
	Backlight:          powersetup.SetBacklightToZero,
	KeyboardBrightness: powersetup.SetKbBrightnessToZero,
	Audio:              powersetup.Mute,
	Bluetooth:          powersetup.DisableBluetoothInterfaces,
	Multicast:          powersetup.DisableMulticast,
}

func initChromeVideoPowerFixtures() {
	// TODO(b/333930542): Remove the fixture with VCD in utility process when it is
	// the default.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoPower",
		Desc:     "Logged into a user session with a condition that logging is enabled and unnecessary or unstable utilities are disabled for power evaluation",
		Contacts: []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		Impl: powersetup.NewPowerUIFixture(MinPowerTestOptions, powersetup.PowerFixtureOptions{
			BrowserType: browser.TypeAsh,
			BrowserExtraOpts: []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
			},
		}),
		Parent:          "gpuWatchHangs",
		SetUpTimeout:    powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoPowerWithVCDInUtilityProcess",
		Desc:         "Similar to chromeVideoPower fixture but running VCD in the utility process",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Impl: powersetup.NewPowerUIFixture(MinPowerTestOptions, powersetup.PowerFixtureOptions{
			BrowserType: browser.TypeAsh,
			BrowserExtraOpts: []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.DisableFeatures("RunVideoCaptureServiceInBrowserProcess"),
			},
		}),
		Parent:          "gpuWatchHangs",
		SetUpTimeout:    powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoPowerLacros",
		Desc:         "Logged into a user session with a condition that logging is enabled (lacros) and unnecessary or unstable utilities are disabled for power evaluation",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Impl: powersetup.NewPowerUIFixture(MinPowerTestOptions, powersetup.PowerFixtureOptions{
			BrowserType: browser.TypeLacros,
			BrowserExtraOpts: []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...),
			},
		}),
		Parent:          "gpuWatchHangs",
		SetUpTimeout:    powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoPowerLacrosWithVCDInUtilityProcess",
		Desc:         "Similar to chromeVideoPowerLacros fixture but running VCD in the utility process",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Impl: powersetup.NewPowerUIFixture(MinPowerTestOptions, powersetup.PowerFixtureOptions{
			BrowserType: browser.TypeLacros,
			BrowserExtraOpts: []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...),
				chrome.DisableFeatures("RunVideoCaptureServiceInBrowserProcess"),
			},
		}),
		Parent:          "gpuWatchHangs",
		SetUpTimeout:    powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})
}
