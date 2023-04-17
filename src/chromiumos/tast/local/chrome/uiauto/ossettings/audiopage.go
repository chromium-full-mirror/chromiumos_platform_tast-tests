// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

var (
	// OSAudioSettingsOutputMuteButton output mute button finder.
	OSAudioSettingsOutputMuteButton = nodewith.Role(role.ToggleButton).NameContaining("Volume").First()

	// OSAudioSettingsOutputVolumeSlider output volume slider finder.
	OSAudioSettingsOutputVolumeSlider = nodewith.Role(role.Slider).NameContaining("Volume").First()

	// OSAudioSettingsOutputDeviceDropdown output device dropdown finder.
	OSAudioSettingsOutputDeviceDropdown = nodewith.Role(role.ComboBoxSelect).NameContaining("Output").First()

	// OSAudioSettingsInputMuteButton input mute button finder.
	OSAudioSettingsInputMuteButton = nodewith.Role(role.ToggleButton).NameContaining("Volume").Nth(1)

	// OSAudioSettingsInputVolumeSlider input volume slider finder.
	OSAudioSettingsInputVolumeSlider = nodewith.Role(role.Slider).NameContaining("Input").First()

	// OSAudioSettingsInputDeviceDropdown input device dropdown finder.
	OSAudioSettingsInputDeviceDropdown = nodewith.Role(role.ComboBoxSelect).NameContaining("Input").First()
)

// LaunchOsSettingsAudioPageFromQuickSettings opens the OS Settings SWA to the
// audio subpage by clicking the settings button on the AudioDetailedView found
// in Quick Settings.
func LaunchOsSettingsAudioPageFromQuickSettings(ctx context.Context, tconn *chrome.TestConn) error {
	// Attempt to open quick settings to audio detailed settings view
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to open Quick Settings")
	}
	if err := quicksettings.OpenAudioSettings(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to open Quick Settings audio detail view")
	}
	settingsGearIcon := nodewith.HasClass("IconButton").Name("Audio settings")
	qsAudioDetailedView := nodewith.HasClass("AudioDetailedView")
	// Click on settings gear to open OS Settings at audio settings subpage.
	ui := uiauto.New(tconn).WithTimeout(2 * time.Second)
	if err := uiauto.Combine("click the Audio settings",
		ui.LeftClick(settingsGearIcon),
		ui.WaitUntilGone(qsAudioDetailedView),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click settings gear in audio detail view")
	}

	testing.ContextLog(ctx, "Waiting for settings app shown in shelf")
	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
		return errors.Wrap(err, "settings app did not open within 1 minute")
	}

	osAudioSettingsWindow := nodewith.Role(role.RootWebArea).NameContaining("Settings - Audio").First()
	testing.ContextLog(ctx, "Waiting for settings app to load audio page")
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(osAudioSettingsWindow)(ctx); err != nil {
		return errors.Wrap(err, "settings app did not open to audio page")
	}

	return nil
}
