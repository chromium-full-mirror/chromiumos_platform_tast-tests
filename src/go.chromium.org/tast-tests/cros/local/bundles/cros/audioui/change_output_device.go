// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/dropdown"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	oss "go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChangeOutputDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "OS Settings SWA launches to audio subpage and verifies changing output device in UI reflected in CRAS",
		// ChromeOS > Software > System Services > Peripherals > Audio (1321112)
		BugComponent: "b:1321112",
		Contacts: []string{
			"cros-peripherals@google.com",
			"ashleydp@google.com",
			"zentaro@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.AloopLoaded{Channels: 2}.Instance(),
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

// ChangeOutputDevice verifies changing output device in UI reflected in CRAS.
func ChangeOutputDevice(ctx context.Context, s *testing.State) {
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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	if err := oss.LaunchOsSettingsAudioPageFromQuickSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open OS Settings audio page from Quick Settings: ", err)
	}
	// Ensure quick settings closed at the end of the test.
	defer quicksettings.Hide(cleanupCtx, tconn)

	s.Log("Verified settings opened to audio settings page")

	if err := verifyActiveOutputNodeChanged(ctx, tconn); err != nil {
		s.Fatal("Failed to verify change active output device behavior: ", err)
	}
	s.Log("Verified change active output device behavior")
}

// verifyActiveOutputNodeChanged verifies changing selected output device also updates CRAS.
func verifyActiveOutputNodeChanged(ctx context.Context, tconn *chrome.TestConn) error {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to start cras")
	}

	// Set expected active node.
	speakerLabel := "Speaker (internal)"
	speakerNode, err := cras.GetNodeByType(ctx, "INTERNAL_SPEAKER")
	if err != nil {
		return errors.Wrap(err, "failed to get INTERNAL_SPEAKER")
	} else if !speakerNode.Active {
		if err := quicksettings.SelectAudioOption(ctx, tconn, speakerLabel); err != nil {
			return errors.Wrapf(err, "failed to set %q as active audio option", speakerLabel)
		}
	}
	testing.ContextLogf(ctx, "Active output node set to: %q", speakerNode.Type)

	ui := uiauto.New(tconn).WithTimeout(2 * time.Second)

	isSelected, err := dropdown.IsSelected(ctx, tconn, oss.OSAudioSettingsOutputDeviceDropdown, speakerLabel)
	if err != nil {
		return errors.Wrap(err, "failed to lookup initial selection")
	}
	if !isSelected {
		return errors.Errorf("failed to verify option with label: (%q) is selected", speakerLabel)
	}

	// Change active node to loopback.
	loopbackLabel := "Loopback Playback"
	speakerOption := nodewith.NameContaining(speakerLabel).Ancestor(oss.OSAudioSettingsOutputDeviceDropdown)
	loopbackOption := nodewith.NameContaining(loopbackLabel).Ancestor(oss.OSAudioSettingsOutputDeviceDropdown)
	if err := uiauto.Combine("Select ALSA_LOOPBACK output device",
		ui.EnsureFocused(oss.OSAudioSettingsOutputDeviceDropdown),
		ui.DoDefault(oss.OSAudioSettingsOutputDeviceDropdown),
		ui.WaitUntilExists(oss.OSAudioSettingsOutputDeviceDropdown.State(state.Collapsed, false)),
		// Verify expected nodes listed in dropdown.
		ui.WaitUntilExists(speakerOption),
		ui.WaitUntilExists(loopbackOption),
		// Select loopback.
		ui.LeftClick(loopbackOption),
		ui.WaitUntilGone(oss.OSAudioSettingsOutputDeviceDropdown.State(state.Collapsed, false)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed select internal speaker in dropdown")
	}

	// Confirm UI updated to show loopback as active node.
	isSelected, err = dropdown.IsSelected(ctx, tconn, oss.OSAudioSettingsOutputDeviceDropdown, loopbackLabel)
	if err != nil {
		return errors.Wrap(err, "failed to lookup selection")
	}
	if !isSelected {
		return errors.Errorf("failed to verify option with label: (%q) is selected", loopbackLabel)
	}
	loopbackNode, err := cras.GetNodeByType(ctx, "ALSA_LOOPBACK")
	if err != nil {
		return errors.Wrap(err, "failed to get loopback")
	} else if !loopbackNode.Active {
		return errors.New("failed to verify ALSA_LOOPBACK node active in CRAS")
	}
	testing.ContextLogf(ctx, "Active output node set to: %q", loopbackNode.Type)
	return nil
}
