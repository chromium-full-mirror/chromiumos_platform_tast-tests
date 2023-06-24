// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	oss "go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChangeOutputVolume,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "OS Settings SWA launches to audio subpage and verifies changing output volume in UI reflected in CRAS",
		// ChromeOS > Software > System Services > Peripherals > Audio (1321112)
		BugComponent: "b:1321112",
		Contacts: []string{
			"cros-peripherals@google.com",
			"ashleydp@google.com",
			"zentaro@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.Microphone()),
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-816eefa8-76ad-43ec-8300-c747f4b59987",
			},
		},
	})
}

// ChangeOutputVolume verifies changing output volume in UI reflected in CRAS.
func ChangeOutputVolume(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Open chrome with audio settings enabled.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("AudioSettingsPage", "QsRevamp"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	// Close test instance of Chrome.
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	cleanup, err := quicksettings.Init(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to init quicksettings: ", err)
	}
	defer cleanup()

	if err := oss.LaunchOsSettingsAudioPageFromQuickSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open OS Settings audio page from Quick Settings: ", err)
	}
	// Ensure quick settings closed at the end of the test.
	defer quicksettings.Hide(cleanupCtx, tconn)

	s.Log("Verified settings opened to audio settings page")

	if err := verifyVolumeChanged(ctx, tconn); err != nil {
		s.Fatal("Failed to verify output volume behavior: ", err)
	}
	s.Log("Verified audio volume behavior")
}

// verifyVolumeChanged confirms that changing volume in UI updates CRAS.
func verifyVolumeChanged(ctx context.Context, tconn *chrome.TestConn) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Set up the keyboard, which is used to increment/decrement the slider.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create audio volume helper")
	}

	// Get current volume.
	prevVol, err := vh.GetVolume(ctx)
	if err != nil {
		return errors.Wrap(err, "failed get output volume")
	}

	// Reset volume to initial state.
	defer func(ctx context.Context, vh *audio.Helper, volume int) {
		if err := vh.SetVolume(ctx, volume); err != nil {
			testing.ContextLog(ctx, "Failed to reset output volume: ", err)
		}
	}(cleanupCtx, vh, prevVol)

	prevVol = 0
	if err := vh.SetVolume(ctx, prevVol); err != nil {
		return errors.Wrap(err, "failed to set initial volume")
	}
	testing.ContextLog(ctx, "Volume set to zero")

	ui := uiauto.New(tconn).WithTimeout(2 * time.Second)

	const (
		// Key to increase slider using keyboard.
		increaseDir string = "right"
		// Key to decrease slider using keyboard.
		decreaseDir string = "left"
		// Slider minimum value.
		sliderMin = 0
		// Slider maximum value.
		sliderMax = 100
		// Slider increments in steps of 10.
		steps = 10
	)

	// Test output volume changed using volume slider updates to expected final value.
	testSlider := func(keyPress string, prevVol, finalVol int) error {
		if err := vh.VerifyVolumeChanged(ctx, func() error {
			if err := ui.EnsureFocused(oss.OSAudioSettingsOutputVolumeSlider)(ctx); err != nil {
				return errors.Wrap(err, "failed to move volume slider")
			}
			if err := uiauto.Repeat(steps, kb.AccelAction(keyPress))(ctx); err != nil {
				return errors.Wrapf(err, "failed to press key: %s", keyPress)
			}

			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to increase volume")
		}

		if vol, err := vh.GetVolume(ctx); err != nil {
			return errors.Wrap(err, "failed get output volume")
		} else if keyPress == increaseDir && vol < prevVol {
			return errors.Wrapf(err, "failed to increase (vol: %d < prevVol: %d)", vol, prevVol)
		} else if keyPress == decreaseDir && vol >= prevVol {
			return errors.Wrapf(err, "failed to decrease (vol: %d > prevVol: %d)", vol, prevVol)
		} else if vol != finalVol {
			return errors.Wrapf(err, "failed to match expected value  !(vol: %d == expected: %d)", vol, finalVol)
		}

		return nil
	}

	// Increase volume to 100.
	if err := testSlider(increaseDir, sliderMin, sliderMax); err != nil {
		return errors.Wrapf(err, "failed to increase slider %p with key press %s ", oss.OSAudioSettingsOutputVolumeSlider, increaseDir)
	}

	// Decrease volume to 0.
	if err := testSlider(decreaseDir, sliderMax, sliderMin); err != nil {
		return errors.Wrapf(err, "failed to decrease slider %p with key press %s ", oss.OSAudioSettingsOutputVolumeSlider, decreaseDir)
	}
	return nil
}
