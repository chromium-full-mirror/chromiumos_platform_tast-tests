// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pnp provides fixture for stable power and performance evaluation.
package pnp

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// StablePowerNoUI provides fixture with no UI.
	StablePowerNoUI = "stablePowerNoUI"
	// StablePowerAsh provides fixture with ash chrome.
	StablePowerAsh = "stablePowerAsh"
	// StablePowerAshGAIA provides fixture with ash chrome with GAIA login.
	StablePowerAshGAIA = "stablePowerAshGAIA"
	// StablePowerAshGAIAFakeHAL provides fixture with ash chrome with GAIA login and use fake HAL.
	StablePowerAshGAIAFakeHAL = "stablePowerAshGAIAFakeHAL"
	// StablePowerLacros provides fixture with lacros chrome.
	StablePowerLacros = "stablePowerLacros"
	// StablePowerLacrosGAIA provides fixture with lacros chrome with GAIA login.
	StablePowerLacrosGAIA = "stablePoweracrosGAIA"
	// StablePowerLacrosGAIAFakeHAL provides fixture with lacros chrome with GAIA login and use fake HAL.
	StablePowerLacrosGAIAFakeHAL = "stablePowerLacrosGAIAFakeHAL"
	// StablePowerLacrosWithSuperResDisabled provides fixture with lacros chrome with GAIA login and force disable super resolution.
	StablePowerLacrosWithSuperResDisabled = "stablePowerLacrosWithSuperResDisabled"
	// StablePowerLacrosWithDigitalZoomSuperResDisabled provides fixture with lacros chrome with GAIA login and force disable digital zoom and super resolution.
	StablePowerLacrosWithDigitalZoomSuperResDisabled = "stablePowerLacrosWithDigitalZoomSuperResDisabled"

	// cameraService is the parent fixture of the StablePower fixtures.
	cameraService = "cameraService"

	fakeHALImageInput = "generic-person-office.jpg"

	// PNPWarmUpTime is the time of the warm up phase of the PNP routine.
	PNPWarmUpTime = 15 * time.Second
)

// PNPTimeParams provides the probing frequency and total time.
var PNPTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 1 * time.Minute}

// EffectsParams holds the config for toggling video conferencing panel.
type EffectsParams struct {
	BlurLevel      vctray.BackgroundBlurLevel
	RelightEnabled bool
}

// MinPowerTestOptions provides an unified stable power environment.
var MinPowerTestOptions = powersetup.PowerTestOptions{
	Wifi:               powersetup.DisableWifiInterfaces,
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

// When using powersetup.NewPowerNoUIFIxture, the night light and darktheme
// should not be toggled or it will generate errors. We assume the night light
// will be off and use light theme.
var minPowerNoUITestOptions = powersetup.PowerTestOptions{
	Wifi:               powersetup.DisableWifiInterfaces,
	NightLight:         powersetup.DoNotDisableNightLight,
	DarkTheme:          powersetup.DoNotChangeTheme,
	UI:                 powersetup.DoNotChangeUI,
	Ramfs:              powersetup.DoNotSetupRamfs,
	Powerd:             powersetup.DisablePowerd,
	UpdateEngine:       powersetup.DisableUpdateEngine,
	VNC:                powersetup.DisableVNC,
	Avahi:              powersetup.DisableAvahi,
	DPTF:               powersetup.DisableDPTF,
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
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl:            &cameraServiceFixture{},
		SetUpTimeout:    5 * time.Second,
		ResetTimeout:    1 * time.Second,
		TearDownTimeout: 1 * time.Second,
		PreTestTimeout:  1 * time.Second,
		PostTestTimeout: 1 * time.Second,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         StablePowerNoUI,
		Desc:         "Disable unnessary or unstable utilities for power evaluation as much as possible",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl: powersetup.NewPowerNoUIFixture(
			minPowerNoUITestOptions,
		),
		Parent:          cameraService,
		SetUpTimeout:    powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         StablePowerAsh,
		Desc:         "Disable unnessary or unstable utilities for power evaluation as much as possible using Ash chrome",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl: powersetup.NewPowerUIFixture(
			MinPowerTestOptions,
			powersetup.PowerFixtureOptions{
				BrowserType:     browser.TypeAsh,
				EnableGAIALogin: false,
			}),
		Parent:          cameraService,
		SetUpTimeout:    powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         StablePowerAshGAIA,
		Desc:         "Disable unnessary or unstable utilities for power evaluation as much as possible using Ash Chrome with GAIA login",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl: powersetup.NewPowerUIFixture(
			MinPowerTestOptions,
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
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
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
		Name:         StablePowerLacros,
		Desc:         "Disable unnessary or unstable utilities for power evaluation as much as possible using Lacros chrome",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl: powersetup.NewPowerUIFixture(
			MinPowerTestOptions,
			powersetup.PowerFixtureOptions{
				BrowserType:     browser.TypeLacros,
				EnableGAIALogin: false,
			}),
		Parent:          cameraService,
		SetUpTimeout:    powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         StablePowerLacrosGAIA,
		Desc:         "Disable unnessary or unstable utilities for power evaluation as much as possible using Lacros chrome with GAIA login",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl: powersetup.NewPowerUIFixture(
			MinPowerTestOptions,
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
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Data:            []string{fakeHALImageInput},
		Impl:            &fakeHALFixture{},
		Parent:          StablePowerLacrosGAIA,
		SetUpTimeout:    5 * time.Second,
		ResetTimeout:    1 * time.Second,
		TearDownTimeout: 1 * time.Second,
		PreTestTimeout:  1 * time.Second,
		PostTestTimeout: 1 * time.Second,
	})
	// TODO(b/225112054): Remove this fixture once enough performance data is collected and the performance is stabilized.
	testing.AddFixture(&testing.Fixture{
		Name:         StablePowerLacrosWithSuperResDisabled,
		Desc:         "Disable unnessary or unstable utilities for power evaluation as much as possible using Lacros chrome and force disable super resolution",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl: powersetup.NewPowerUIFixture(
			MinPowerTestOptions,
			powersetup.PowerFixtureOptions{
				BrowserType:     browser.TypeLacros,
				EnableGAIALogin: false,
				BrowserExtraOpts: []chrome.Option{
					chrome.ExtraArgs("--camera-super-res-override=force-disabled"),
				},
			}),
		Parent:          cameraService,
		SetUpTimeout:    powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})
	// TODO(b/225112054): Remove this fixture once enough performance data is collected and the performance is stabilized.
	testing.AddFixture(&testing.Fixture{
		Name:         StablePowerLacrosWithDigitalZoomSuperResDisabled,
		Desc:         "Disable unnessary or unstable utilities for power evaluation as much as possible using Lacros chrome and force disable digital zoom and super resolution",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl: powersetup.NewPowerUIFixture(
			MinPowerTestOptions,
			powersetup.PowerFixtureOptions{
				BrowserType:     browser.TypeLacros,
				EnableGAIALogin: false,
				BrowserExtraOpts: []chrome.Option{
					chrome.DisableFeatures("CameraAppDigitalZoom"),
					chrome.ExtraArgs("--camera-super-res-override=force-disabled"),
				},
			}),
		Parent:          cameraService,
		SetUpTimeout:    powersetup.SetUpTimeout,
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
		s.Fatal("Failed to set up camera test config: ", err)
	}

	fakeCameraConfig := testutil.FakeCameraConfig{
		ID:        1,
		Connected: true,
		Frames:    &testutil.FakeCameraImageConfig{Path: dutFakeHALPath},
		SupportedFormats: []*testutil.FakeCameraFormatsConfig{{
			Width:      1280,
			Height:     720,
			FrameRates: []int{30}}},
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

// Cooldown should be run before loading the work for cooldown phase.
func Cooldown(ctx context.Context) error {
	testing.ContextLog(ctx, "[Cool Down Phase]")
	if err := power.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "cooldown failed")
	}

	return nil
}

// WarmUp should be run immediately after the work is loaded for warm up phase.
func WarmUp(ctx context.Context) error {
	testing.ContextLog(ctx, "[Warm Up Phase] Start warming up for ", PNPWarmUpTime)
	// GoBigSleepLint: Warming up.
	if err := testing.Sleep(ctx, PNPWarmUpTime); err != nil {
		return errors.Wrap(err, "failed to sleep to warm up")
	}
	return nil
}

// Routine provides a standard way of running PNP measurement.
type Routine struct {
	rec *power.Recorder
}

// MeasurePower records the power and saves the data.
func (f *Routine) MeasurePower(ctx, cleanupCtx context.Context, outDir, testName string, upload bool) error {
	testing.ContextLog(ctx, "[Record Phase] Start recording trace for ", PNPTimeParams.Total)
	f.rec = power.NewRecorder(ctx, PNPTimeParams.Interval, outDir, testName)

	if err := f.rec.Start(ctx); err != nil {
		return errors.Wrap(err, "cannot start collecting power metrics")
	}

	// GoBigSleepLint: Collect power metrics.
	if err := testing.Sleep(ctx, PNPTimeParams.Total); err != nil {
		return errors.Wrap(err, "failed to sleep to collect power metrics")
	}

	if upload {
		if err := f.rec.Finish(ctx); err != nil {
			return errors.Wrap(err, "cannot finish collecting power metrics")
		}
	} else {
		if _, err := f.rec.Stop(ctx); err != nil {
			return errors.Wrap(err, "cannot stop collecting power metrics")
		}

	}
	return nil
}

// UploadMetrics uploads the data and closes the recorder.
func (f *Routine) UploadMetrics(ctx, cleanupCtx context.Context, extraPerfValues ...*perf.Values) error {
	if err := f.rec.Finish(ctx, extraPerfValues...); err != nil {
		return errors.Wrap(err, "cannot finish collecting power metrics")
	}
	return nil
}

// Close closes the owned recorder.
func (f *Routine) Close(cleanupCtx context.Context) error {
	if f.rec != nil {
		return f.rec.Close(cleanupCtx)
	}
	return nil
}
