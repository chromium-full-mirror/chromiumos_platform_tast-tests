// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RenderAudioPage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "OS Settings SWA launches to audio subpage and renders components",
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

// RenderAudioPage verifies audio settings can be opened from quick settings.
func RenderAudioPage(ctx context.Context, s *testing.State) {
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

	if err := launchOsSettingsAudioPageFromQuickSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open OS Settings audio page from Quick Settings: ", err)
	}
	// Ensure quick settings closed at the end of the test.
	defer quicksettings.Hide(cleanupCtx, tconn)

	s.Log("Verified settings opened to audio settings page")

	if err := verifyLayoutComponents(ctx, tconn); err != nil {
		s.Fatal("Failed to verify audio setting page layout: ", err)
	}
	s.Log("Verified audio settings layout")
}

// launchOsSettingsAudioPageFromQuickSettings opens the OS Settings SWA to the
// audio subpage by clicking the settings button on the AudioDetailedView found
// in Quick Settings.
func launchOsSettingsAudioPageFromQuickSettings(ctx context.Context, tconn *chrome.TestConn) error {
	// Attempt to open quick settings to audio detailed settings view
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to open Quick Settings")
	}
	if err := quicksettings.OpenAudioSettings(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to open Quick Settings audio detail view")
	}
	settingsGearIcon := nodewith.HasClass("IconButton").Name("Audio settings")
	qsAudioDetailedView := nodewith.ClassName("AudioDetailedView")
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

// verifyLayoutComponents tests if audio input and output sections are displayed.
func verifyLayoutComponents(ctx context.Context, tconn *chrome.TestConn) error {
	// TODO(b/260277007): Update labels when strings finalized.
	outputDeviceDropdown := nodewith.Role(role.ComboBoxSelect).NameContaining("Output").First()
	outputMuteButton := nodewith.Role(role.ToggleButton).NameContaining("Volume").First()
	outputVolumeSlider := nodewith.Role(role.Slider).NameContaining("Volume").First()
	inputDeviceDropdown := nodewith.Role(role.ComboBoxSelect).NameContaining("Input").First()
	inputMuteButton := nodewith.Role(role.ToggleButton).NameContaining("Gain").First()
	inputVolumeSlider := nodewith.Role(role.Slider).NameContaining("Input").First()

	ui := uiauto.New(tconn)

	for _, node := range []*nodewith.Finder{
		outputDeviceDropdown,
		outputMuteButton,
		outputVolumeSlider,
		inputDeviceDropdown,
		inputMuteButton,
		inputVolumeSlider,
	} {
		nodeInfo, err := ui.Info(ctx, node)
		if err != nil {
			return errors.Wrap(err, "failed to get node info")
		}
		testing.ContextLogf(ctx, "Found %q %q", nodeInfo.Name, nodeInfo.ClassName)
	}
	return nil
}
