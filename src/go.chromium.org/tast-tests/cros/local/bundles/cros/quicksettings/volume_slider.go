// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VolumeSlider,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the Quick Settings volume slider can be adjusted by keyboard",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"sylvieliu@chromium.org",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-39d4666a-39dc-40ed-a918-75acf57335a0",
		}},
		Fixture:      "chromeLoggedInQsRevampEnabled",
		HardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.SkipOnModel("kakadu", "atlas")),
	})
}

func muteUnmuteVolume(ctx context.Context, tconn *chrome.TestConn, vh *audio.Helper) error {
	mutedButton := nodewith.Name("Toggle Volume. Volume is muted.").Role(role.ToggleButton)
	unmutedButton := nodewith.Name("Toggle Volume. Volume is on, toggling will mute audio.").Role(role.ToggleButton)

	ui := uiauto.New(tconn)

	// Mute the volume.
	if err := ui.WithTimeout(1 * time.Second).LeftClick(unmutedButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the volume toggle")
	}

	mute, err := vh.IsMuted(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check volume mute status")
	}
	if !mute {
		return errors.New("failed to mute the volume")
	}

	// Unmute the volume.
	if err := ui.WithTimeout(1 * time.Second).LeftClick(mutedButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the volume toggle")
	}

	mute, err = vh.IsMuted(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check volume mute status")
	}
	if mute {
		return errors.New("failed to unmute the volume")
	}
	return nil
}

// VolumeSlider tests that the volume slider can be adjusted up and down.
func VolumeSlider(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Set up the keyboard, which is used to increment/decrement the slider.
	// TODO(crbug/1123231): use better slider automation controls if possible, instead of keyboard controls.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show Quick Settings: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Test the volume slider.
	initialVolume, err := quicksettings.SliderValue(ctx, tconn, quicksettings.SliderTypeVolume)
	if err != nil {
		s.Fatal("Failed initial value check for volume slider: ", err)
	}

	decreaseVolume, err := quicksettings.DecreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeVolume)
	if err != nil {
		s.Fatalf("Failed to decrease volume slider from %d: %v", initialVolume, err)
	}
	s.Log("Decreased volume slider value: ", decreaseVolume)

	increaseVolume, err := quicksettings.IncreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeVolume)
	if err != nil {
		s.Fatal("Failed to increase volume slider: ", err)
	}
	s.Log("Increased volume slider value: ", increaseVolume)

	// Test the volume toggle.
	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create the volumeHelper: ", err)
	}
	if err := muteUnmuteVolume(ctx, tconn, vh); err != nil {
		s.Fatal("Failed to mute/unmute volume: ", err)
	}
}
