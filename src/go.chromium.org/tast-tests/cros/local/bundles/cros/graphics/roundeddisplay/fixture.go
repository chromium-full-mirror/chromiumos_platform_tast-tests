// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package roundeddisplay

import (
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "powerAshWithRoundedDisplay",
		Desc: "UI CUJ tests with Rounded Windows feature enabled",
		Contacts: []string{
			"zoraiznaeem@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl: powersetup.NewPowerUIFixture(powersetup.PowerTestOptions{
			NightLight:         powersetup.DisableNightLight,
			DarkTheme:          powersetup.EnableLightTheme,
			KeyboardBrightness: powersetup.SetKbBrightnessToZero,
		}, powersetup.PowerFixtureOptions{
			BrowserType: browser.TypeAsh,
			BrowserExtraOpts: []chrome.Option{
				chrome.ExtraArgs(FormatDisplayPropertiesAsSwitch(PanelRadii{
					TopLeft:     18,
					TopRight:    18,
					BottomLeft:  18,
					BottomRight: 18,
				})),
				chrome.EnableFeatures("RoundedDisplay"),
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
