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
		Func:         ChangeInputDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "OS Settings SWA launches to audio subpage  and verifies changing input device in UI reflected in CRAS",
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

// ChangeInputDevice verifies changing input device in UI reflected in CRAS.
func ChangeInputDevice(ctx context.Context, s *testing.State) {
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

	if err := oss.LaunchOsSettingsAudioPageFromQuickSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open OS Settings audio page from Quick Settings: ", err)
	}
	// Ensure quick settings closed at the end of the test.
	defer quicksettings.Hide(cleanupCtx, tconn)

	s.Log("Verified settings opened to audio settings page")

	if err := verifyActiveInputNodeChanged(ctx, tconn); err != nil {
		s.Fatal("Failed to verify change active input device behavior: ", err)
	}
	s.Log("Verified change active input device behavior")
}

// verifyActiveInputNodeChanged verifies changing selected input device also updates CRAS.
func verifyActiveInputNodeChanged(ctx context.Context, tconn *chrome.TestConn) error {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to start cras")
	}

	// Set expected active node.
	microphoneLabel := "Microphone (internal)"
	microphoneNode, err := cras.GetNodeByType(ctx, "INTERNAL_MIC")
	if err != nil {
		return errors.Wrap(err, "failed to get INTERNAL_MIC")
	}
	if !microphoneNode.Active {
		if err := quicksettings.SelectAudioOption(ctx, tconn, microphoneLabel); err != nil {
			return errors.Wrapf(err, "failed to set %q as active audio option", microphoneLabel)
		}
	}
	testing.ContextLogf(ctx, "Active input node set to: %q", microphoneNode.Type)

	ui := uiauto.New(tconn).WithTimeout(2 * time.Second)

	// Verify the selected value matches the expected audio node label.
	isSelected, err := dropdown.IsSelected(ctx, tconn, oss.OSAudioSettingsInputDeviceDropdown, microphoneLabel)
	if err != nil {
		return errors.Wrap(err, "failed to lookup initial selection")
	}
	if !isSelected {
		return errors.Errorf("failed to verify option with label: (%q) is selected", microphoneLabel)
	}

	// Change active node to loopback mic.
	loopbackLabel := "Loopback Capture"
	microphoneOption := nodewith.NameContaining(microphoneLabel).Ancestor(oss.OSAudioSettingsInputDeviceDropdown)
	loopbackOption := nodewith.NameContaining(loopbackLabel).Ancestor(oss.OSAudioSettingsInputDeviceDropdown)

	if err := uiauto.Combine("Select ALSA_LOOPBACK input device",
		ui.EnsureFocused(oss.OSAudioSettingsInputDeviceDropdown),
		ui.DoDefault(oss.OSAudioSettingsInputDeviceDropdown),
		ui.WaitUntilExists(oss.OSAudioSettingsInputDeviceDropdown.State(state.Collapsed, false)),
		// Verify expected nodes listed in dropdown.
		ui.WaitUntilExists(microphoneOption),
		ui.WaitUntilExists(loopbackOption),
		// Select loopback.
		ui.LeftClick(loopbackOption),
		ui.WaitUntilGone(oss.OSAudioSettingsInputDeviceDropdown.State(state.Collapsed, false)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed select ALSA_LOOPBACK in dropdown")
	}

	// Confirm UI updated to show loopback as active node.
	isSelected, err = dropdown.IsSelected(ctx, tconn, oss.OSAudioSettingsInputDeviceDropdown, loopbackLabel)
	if err != nil {
		return errors.Wrap(err, "failed to lookup dropdown selection")
	}
	if !isSelected {
		return errors.Errorf("failed to verify option with label: (%q) is selected", loopbackLabel)
	}

	loopbackNode, err := cras.GetNodeByMatcher(ctx, audio.MatchNodeTypeDirection{
		Type:      "ALSA_LOOPBACK",
		Direction: audio.InputStream,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get ALSA_LOOPBACK node")
	}
	if !loopbackNode.Active {
		return errors.New("failed to verify ALSA_LOOPBACK node active in CRAS")
	}
	testing.ContextLogf(ctx, "Active input node set to: %q", loopbackNode.Type)
	return nil
}
