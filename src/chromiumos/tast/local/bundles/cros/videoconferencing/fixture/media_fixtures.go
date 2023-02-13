// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"os"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/camera/testutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/input/voice"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

// List of fixture names for ML service testing.
const (
	LoggedInWithFakeHALAndEffectsEnabled      = "loggedInWithFakeHALAndEffectsEnabled"
	LoggedInWithFakeHALAndEffectsDisabled     = "loggedInWithFakeHALAndEffectsDisabled"
	GAIALoggedInWithFakeHALAndEffectsEnabled  = "gaiaLoggedInWithFakeHALAndEffectsEnabled"
	GAIALoggedInWithFakeHALAndEffectsDisabled = "gaiaLoggedInWithFakeHALAndEffectsDisabled"

	GAIALoggedInClamshellWithFakeHALAndEffectsEnabled       = "gaiaLoggedInClamshellWithFakeHALAndEffectsEnabled"
	GAIALoggedInTabletWithFakeHALAndEffectsEnabled          = "gaiaLoggedInTabletWithFakeHALAndEffectsEnabled"
	GAIALoggedInLacrosClamshellWithFakeHALAndEffectsEnabled = "gaiaLoggedInLacrosClamshellWithFakeHALAndEffectsEnabled"
	GAIALoggedInLacrosTabletWithFakeHALAndEffectsEnabled    = "gaiaLoggedInLacrosTabletWithFakeHALAndEffectsEnabled"
)

type micType int

const (
	internalMic micType = iota
	aloop
)

type platformEffectLevel int

const (
	platformEffectDefault platformEffectLevel = iota
	platformEffectEnabled
	platformEffectDisabled
)

type cameraConfig struct {
	cameraType     testutil.UseCameraType
	platformEffect platformEffectLevel
}

// Available configurations of camera.
var (
	halCameraWithPlatformEffectsEnabled = cameraConfig{
		cameraType:     testutil.UseFakeHALCamera,
		platformEffect: platformEffectEnabled,
	}

	halCameraWithPlatformEffectsDisabled = cameraConfig{
		cameraType:     testutil.UseFakeHALCamera,
		platformEffect: platformEffectDisabled,
	}
)

const (
	// Files checked to force disable/enable EffectsStreamManipulator.
	platformEffectsForceDisablePath = "/run/camera/force_disable_effects"
	platformEffectsForceEnablePath  = "/run/camera/force_enable_effects"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: LoggedInWithFakeHALAndEffectsEnabled,
		Desc: "A fixture with fake user logged in using fake HAL camera with platform effects enabled",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            mediaSetupFixture(internalMic, halCameraWithPlatformEffectsEnabled),
		Parent:          LoggedIn,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: LoggedInWithFakeHALAndEffectsDisabled,
		Desc: "A fixture with fake user logged in using fake HAL camera with platform effects enabled",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            mediaSetupFixture(internalMic, halCameraWithPlatformEffectsDisabled),
		Parent:          LoggedIn,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInWithFakeHALAndEffectsEnabled,
		Desc: "A fixture with gaia user logged in using fake HAL camera with platform effects enabled",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            mediaSetupFixture(internalMic, halCameraWithPlatformEffectsEnabled),
		Parent:          GAIALoggedIn,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInWithFakeHALAndEffectsDisabled,
		Desc: "A fixture with gaia user logged in using fake HAL camera with platform effects disabled",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            mediaSetupFixture(internalMic, halCameraWithPlatformEffectsDisabled),
		Parent:          GAIALoggedIn,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInClamshellWithFakeHALAndEffectsEnabled,
		Desc: "A fixture with gaia user logged in clamshell mode using fake HAL camera with platform effects enabled",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            mediaSetupFixture(internalMic, halCameraWithPlatformEffectsEnabled),
		Parent:          GAIALoggedInClamshell,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInTabletWithFakeHALAndEffectsEnabled,
		Desc: "A fixture with gaia user logged in tablet mode using fake media devices with platform effects enabled",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            mediaSetupFixture(internalMic, halCameraWithPlatformEffectsEnabled),
		Parent:          GAIALoggedInTablet,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInLacrosClamshellWithFakeHALAndEffectsEnabled,
		Desc: "A fixture with gaia user logged in Lacros clamshell mode using fake HAL camera with platform effects enabled",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            mediaSetupFixture(internalMic, halCameraWithPlatformEffectsEnabled),
		Parent:          GAIALoggedInLacrosClamshell,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInLacrosTabletWithFakeHALAndEffectsEnabled,
		Desc: "A fixture with gaia user logged in Lacros tablet mode using fake HAL camera with platform effects enabled",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            mediaSetupFixture(internalMic, halCameraWithPlatformEffectsEnabled),
		Parent:          GAIALoggedInLacrosTablet,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

func mediaSetupFixture(mic micType, camConfig cameraConfig) testing.FixtureImpl {
	return &mediaFixtureImpl{mic, camConfig, nil}
}

// mediaFixtureImpl implements testing.FixtureImpl.
type mediaFixtureImpl struct {
	micType   micType
	camConfig cameraConfig
	cleanup   []action.Action // A list of cleanup actions to be executed in teardown.
}

func (f *mediaFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	parentVal := s.ParentValue()
	cr := parentVal.(BaseSetupFixtData).Chrome()

	f.cleanup = []action.Action{}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	if f.micType == aloop {
		testing.ContextLog(ctx, "Setting up Aloop for audio test")
		// Setup CRAS Aloop for audio test.
		cleanup, err := voice.EnableAloop(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to enable Aloop: ", err)
		}

		f.cleanup = append(f.cleanup, func(ctx context.Context) error {
			cleanup(ctx)
			return nil
		})
	}

	// Setup camera.
	testing.ContextLogf(ctx, "Setting up camera: %+v", f.camConfig)
	if err := f.setupCamera(ctx); err != nil {
		s.Fatal("Failed to setup camera: ", err)
	}

	return s.ParentValue()
}

func (f *mediaFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *mediaFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {

}

func (f *mediaFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *mediaFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	for _, cleanupFunc := range f.cleanup {
		if err := cleanupFunc(ctx); err != nil {
			s.Error("Failed to cleanup: ", err)
		}
	}
}

func (f *mediaFixtureImpl) setupCamera(ctx context.Context) error {
	// This ensures always restart cros-camera in the end of cleanup camera settings.
	defer func() {
		f.cleanup = append(f.cleanup, func(ctx context.Context) error {
			return upstart.RestartJob(ctx, "cros-camera")
		})
	}()

	if err := f.togglePlatformEffects(ctx, f.camConfig.platformEffect); err != nil {
		return errors.Wrapf(err, "failed to set platform effects: %v", f.camConfig.platformEffect)
	}

	if err := testutil.SetupTestConfig(ctx, f.camConfig.cameraType); err != nil {
		return errors.Wrap(err, "failed to setup camera test config")
	}
	f.cleanup = append(f.cleanup, testutil.RemoveTestConfig)

	if f.camConfig.cameraType == testutil.UseFakeHALCamera {
		if err := testutil.SetupFakeHALConfig(ctx); err != nil {
			return errors.Wrap(err, "failed to configure HAL camera")
		}
		f.cleanup = append(f.cleanup, testutil.RemoveFakeHALConfig)
	}
	if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
		return errors.Wrap(err, "failed to restart cros-camera after setup camera")
	}
	return nil
}

// togglePlatformEffects ensures the switch file exists / gone.
// According to the setting requirement, it creates / removes config file.
// Then append restore function to the fixture cleanup.
func (f *mediaFixtureImpl) togglePlatformEffects(ctx context.Context, platformEffect platformEffectLevel) error {
	ensureFileExists := func(ctx context.Context, filePath string) error {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			file, err := os.Create(filePath)
			if err != nil {
				return errors.Wrapf(err, "failed to create %q", filePath)
			}
			file.Close()
			f.cleanup = append(f.cleanup, func(ctx context.Context) error {
				if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
					return errors.Wrapf(err, "failed to remove %q", filePath)
				}
				return nil
			})
		} else if err != nil {
			return errors.Wrapf(err, "failed to check %q", filePath)
		}
		return nil
	}

	ensureFileGone := func(ctx context.Context, filePath string) error {
		if err := os.Remove(filePath); err == nil {
			// Switch file should be restored in cleanup if it is removed.
			f.cleanup = append(f.cleanup, func(ctx context.Context) error {
				file, err := os.Create(filePath)
				if err != nil {
					return errors.Wrapf(err, "failed to create %q", filePath)
				}
				file.Close()
				return nil
			})
		} else if !os.IsNotExist(err) {
			return errors.Wrapf(err, "failed to remove %q", filePath)
		}
		return nil
	}

	switch f.camConfig.platformEffect {
	// platformEffectsForceEnablePath: Yes; platformEffectsForceDisablePath: No
	case platformEffectEnabled:
		if err := ensureFileGone(ctx, platformEffectsForceDisablePath); err != nil {
			return err
		}
		return ensureFileExists(ctx, platformEffectsForceEnablePath)

	// platformEffectsForceEnablePath: No; platformEffectsForceDisablePath: Yes
	case platformEffectDisabled:
		if err := ensureFileGone(ctx, platformEffectsForceEnablePath); err != nil {
			return err
		}
		return ensureFileExists(ctx, platformEffectsForceDisablePath)

	// platformEffectsForceEnablePath: No; platformEffectsForceDisablePath: No
	default:
		if err := ensureFileGone(ctx, platformEffectsForceEnablePath); err != nil {
			return err
		}
		return ensureFileGone(ctx, platformEffectsForceDisablePath)
	}
}
