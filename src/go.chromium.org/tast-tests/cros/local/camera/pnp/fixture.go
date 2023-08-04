// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pnp provides fixture for stable power and performance evaluation.
package pnp

import (
	"context"
	"time"

	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/upstart"
)

const (
	// StablePowerAshGAIA provide fixture with ash chrome with GAIA login
	StablePowerAshGAIA = "stablePowerAshGAIA"
	// StablePowerLacrosGAIA provide fixture with lacros chrome with GAIA login
	StablePowerLacrosGAIA = "stablePowerLacrosGAIA"

	cameraService = "cameraService"
)

// PNPTimeParams provide the probing frequency and total times.
var PNPTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 60 * time.Second}

var minPowerTestOptions = powersetup.PowerTestOptions{
	Wifi:               powersetup.DisableWifiInterfaces,
	NightLight:         powersetup.DisableNightLight,
	DarkTheme:          powersetup.EnableDarkTheme,
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

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            cameraService,
		Desc:            "Enable necessary utilities for using camera",
		Contacts:        []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		Impl:            &cameraServiceFixture{},
		SetUpTimeout:    5 * time.Second,
		ResetTimeout:    1 * time.Second,
		TearDownTimeout: 1 * time.Second,
		PreTestTimeout:  1 * time.Second,
		PostTestTimeout: 1 * time.Second,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     StablePowerAshGAIA,
		Desc:     "Disable unnessary or unstable utilities for power evaluation as much as possible using Ash Chrome with GAIA login",
		Contacts: []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		Impl: powersetup.NewPowerUIFixture(
			minPowerTestOptions,
			powersetup.PowerFixtureOptions{
				BrowserType:     browser.TypeAsh,
				EnableGAIALogin: true,
			}),
		Parent:          cameraService,
		SetUpTimeout:    chrome.GAIALoginTimeout + powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     StablePowerLacrosGAIA,
		Desc:     "Disable unnessary or unstable utilities for power evaluation as much as possible using Lacros chrome with GAIA login",
		Contacts: []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		Impl: powersetup.NewPowerUIFixture(
			minPowerTestOptions,
			powersetup.PowerFixtureOptions{
				BrowserType:     browser.TypeLacros,
				EnableGAIALogin: true,
			}),
		Parent:          cameraService,
		SetUpTimeout:    chrome.GAIALoginTimeout + powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})
}

type cameraServiceFixture struct{}

func (f *cameraServiceFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera service: ", err)
	}
	return nil
}

func (f *cameraServiceFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *cameraServiceFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *cameraServiceFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *cameraServiceFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
