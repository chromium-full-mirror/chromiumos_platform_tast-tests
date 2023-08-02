// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pnp provides fixture for stable power and performance evaluation.
package pnp

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/upstart"
)

const (
	// StablePowerAshGAIA provide fixture with ash chrome with GAIA login
	StablePowerAshGAIA = "stablePowerAshGAIA"
	// StablePowerAshGAIAFakeHAL provide fixture with ash chrome with GAIA login and use fake HAL
	StablePowerAshGAIAFakeHAL = "stablePowerAshGAIAFakeHAL"
	// StablePowerLacrosGAIA provide fixture with lacros chrome with GAIA login
	StablePowerLacrosGAIA = "stablePowerLacrosGAIA"
	// StablePowerLacrosGAIAFakeHAL provide fixture with lacros chrome with GAIA login and use fake HAL
	StablePowerLacrosGAIAFakeHAL = "stablePowerLacrosGAIAFakeHAL"

	// cameraService is the parent fixture of the StablePower fixtures
	cameraService = "cameraService"

	fakeHALImageInput = "generic-person-office.jpg"
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
		Name:            StablePowerAshGAIAFakeHAL,
		Desc:            "Disable unnessary or unstable utilities for power evaluation as much as possible using Ash Chrome with GAIA login and fake HAL",
		Contacts:        []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		Data:            []string{fakeHALImageInput},
		Impl:            &fakeHALFixture{},
		Parent:          StablePowerAshGAIA,
		SetUpTimeout:    5 * time.Second,
		ResetTimeout:    1 * time.Second,
		TearDownTimeout: 1 * time.Second,
		PreTestTimeout:  1 * time.Second,
		PostTestTimeout: 1 * time.Second,
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
	testing.AddFixture(&testing.Fixture{
		Name:            StablePowerLacrosGAIAFakeHAL,
		Desc:            "Disable unnessary or unstable utilities for power evaluation as much as possible using Lacros chrome with GAIA login and fake HAL",
		Contacts:        []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		Data:            []string{fakeHALImageInput},
		Impl:            &fakeHALFixture{},
		Parent:          StablePowerLacrosGAIA,
		SetUpTimeout:    5 * time.Second,
		ResetTimeout:    1 * time.Second,
		TearDownTimeout: 1 * time.Second,
		PreTestTimeout:  1 * time.Second,
		PostTestTimeout: 1 * time.Second,
	})
}

type cameraServiceFixture struct{}

func (f *cameraServiceFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera service: ", err)
	}
	return nil
}

func (f *cameraServiceFixture) TearDown(ctx context.Context, s *testing.FixtState) {}

func (f *cameraServiceFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *cameraServiceFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *cameraServiceFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

type fakeHALFixture struct {
	cleanup []action.Action // A list of cleanup actions to be executed in teardown.
}

func (f *fakeHALFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	f.cleanup = []action.Action{}

	// This ensures always restart cros-camera in the end of cleanup camera settings.
	defer func() {
		f.cleanup = append(f.cleanup, func(ctx context.Context) error {
			return upstart.RestartJob(ctx, "cros-camera")
		})
	}()

	testing.ContextLog(ctx, "Prepare fake HAL")
	// Fake image input must be placed under camera cache folder,
	// which is a sandbox environment for camera testing.
	dutFakeHALPath, err := testutil.CopyFakeHALFrameImage(s.DataPath(fakeHALImageInput))
	if err != nil {
		s.Fatal("Failed to copy fake camera input: ", err)
	}
	f.cleanup = append(f.cleanup, func(ctx context.Context) error {
		return os.Remove(dutFakeHALPath)
	})

	f.cleanup = append(f.cleanup, testutil.RemoveTestConfig)
	if err := testutil.SetupTestConfig(ctx, testutil.UseFakeHALCamera); err != nil {
		s.Fatal("Failed to setup camera test config: ", err)
	}

	fakeCameraConfig := testutil.FakeCameraConfig{
		ID:        1,
		Connected: true,
		Frames:    &testutil.FakeCameraImageConfig{Path: dutFakeHALPath},
	}
	if err := testutil.WriteFakeHALConfig(ctx,
		testutil.FakeHALConfig{Cameras: []testutil.FakeCameraConfig{fakeCameraConfig}}); err != nil {
		s.Error("Failed to configure HAL camera: ", err)
	}
	f.cleanup = append(f.cleanup, testutil.RemoveFakeHALConfig)

	if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
		s.Error("Failed to restart cros-camera after setup camera: ", err)
	}
	return s.ParentValue()
}

func (f *fakeHALFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	for _, cleanupFunc := range f.cleanup {
		if err := cleanupFunc(ctx); err != nil {
			s.Error("Failed to cleanup: ", err)
		}
	}
}

func (f *fakeHALFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *fakeHALFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fakeHALFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
