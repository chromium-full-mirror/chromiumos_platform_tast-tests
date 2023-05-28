// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"strconv"
	"time"

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
		Func:         ChangeInputVolume,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "OS Settings SWA launches to audio subpage and verifies changing input volume in UI reflected in Quick Settings",
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

// ChangeInputVolume verifies changing output volume in UI reflected in Quick Settings.
func ChangeInputVolume(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Open chrome with audio settings enabled.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("AudioSettingsPage"))
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

	if err := oss.LaunchOsSettingsAudioPageFromQuickSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open OS Settings audio page from Quick Settings: ", err)
	}
	// Ensure quick settings closed at the end of the test.
	defer quicksettings.Hide(cleanupCtx, tconn)

	s.Log("Verified settings opened to audio settings page")

	if err := verifyGainVolumeChanged(ctx, tconn); err != nil {
		s.Fatal("Failed to verify input gain volume behavior: ", err)
	}
	s.Log("Verified gain volume behavior")
}

type sliderDirection string

const (
	increase sliderDirection = "right"
	decrease sliderDirection = "left"
	steps    int             = 10
)

// moveSlider uses keyboard press to move slider to upper or lower limit depending
// on provided key-press direction.
func moveSlider(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, direction sliderDirection) error {
	ui := uiauto.New(tconn)
	if err := ui.EnsureFocused(oss.OSAudioSettingsInputVolumeSlider)(ctx); err != nil {
		return errors.Wrap(err, "failed to focus on slider")
	}
	if err := uiauto.Repeat(steps, kb.AccelAction(string(direction)))(ctx); err != nil {
		return errors.Wrapf(err, "failed to press key %q", direction)
	}
	return nil
}

// sliderValueEquals compares input gain slider value against expected value.
func sliderValueEquals(ctx context.Context, tconn *chrome.TestConn, expected int) error {
	ui := uiauto.New(tconn)
	nodeInfo, err := ui.Info(ctx, oss.OSAudioSettingsInputVolumeSlider)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve gain slider info")
	}
	nodeValue, err := strconv.Atoi(nodeInfo.Value)
	if err != nil {
		return errors.Wrapf(err, "failed to convert gain value (%q) into number", nodeInfo.Value)
	}
	if nodeValue != expected {
		return errors.Errorf("failed to match slider value. Expected: %d, Got: %q", expected, nodeInfo.Value)
	}
	return nil
}

// compareQSSliderValue compares input gain slider value against the QuickSettings.
// mic gain slider.
func compareQSSliderValue(ctx context.Context, tconn *chrome.TestConn) error {
	qsValue, err := quicksettings.SliderValue(ctx, tconn, quicksettings.SliderTypeMicGain)
	if err != nil {
		return errors.Wrap(err, "failed to look up QuickSettings MicGain slider value")
	}
	return sliderValueEquals(ctx, tconn, qsValue)
}

// verifyGainVolumeChanged confirms that changing volume in UI updates Quick Settings.
func verifyGainVolumeChanged(ctx context.Context, tconn *chrome.TestConn) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to setup keyboard")
	}
	defer kb.Close(ctx)

	// Change gain to min value.
	if err := moveSlider(ctx, tconn, kb, decrease); err != nil {
		return errors.Wrap(err, "failed to move slider toward zero")
	}
	expected := 0
	if err := sliderValueEquals(ctx, tconn, expected); err != nil {
		return errors.Wrapf(err, "failed to match expected value (%q)", expected)
	}
	if err := compareQSSliderValue(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to match quick settings and audio settings slider value")
	}

	// Change gain to max value.
	if err := moveSlider(ctx, tconn, kb, increase); err != nil {
		return errors.Wrap(err, "failed to move slider toward 100")
	}
	expected = 100
	if err := sliderValueEquals(ctx, tconn, expected); err != nil {
		return errors.Wrapf(err, "failed to match expected value (%q)", expected)
	}
	if err := compareQSSliderValue(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to match quick settings and audio settings slider value")
	}

	return nil
}
