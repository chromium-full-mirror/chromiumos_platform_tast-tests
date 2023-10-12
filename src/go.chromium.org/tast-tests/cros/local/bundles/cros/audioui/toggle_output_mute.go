// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	oss "go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ToggleOutputMute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "OS Settings SWA launches to audio subpage and verifies toggling output mute in UI reflected in CRAS",
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

// ToggleOutputMute verifies output mute state can be updated from UI.
func ToggleOutputMute(ctx context.Context, s *testing.State) {
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

	if err := verifyVolumeMuteChanged(ctx, tconn); err != nil {
		s.Fatal("Failed to verify output mute behavior: ", err)
	}
	s.Log("Verified audio mute behavior")
}

// verifyVolumeMuteChanged confirms that changing output mute in UI also updates CRAS.
func verifyVolumeMuteChanged(ctx context.Context, tconn *chrome.TestConn) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	// Ensure device un-muted at end of test.
	defer crastestclient.Unmute(cleanupCtx)

	testing.ContextLog(ctx, "Ensure device is not muted")
	if err := crastestclient.Unmute(ctx); err != nil {
		return errors.Wrap(err, "failed to set output as not muted")
	}

	// Record initial mute state and verify start not muted.
	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to setup audio volume helper")
	}

	unmutedState, err := vh.IsMuted(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to lookup audio mute state")
	} else if unmutedState {
		return errors.Wrap(err, "failed to initialize output to not muted")
	}

	boolAsChecked := func(state bool) checked.Checked {
		if state {
			return checked.True
		}
		return checked.False
	}

	ui := uiauto.New(tconn)

	muteChecked := func(expected checked.Checked) error {
		if nodeInfo, err := ui.Info(ctx, oss.OSAudioSettingsOutputMuteButton); err != nil {
			return errors.Wrap(err, "failed to lookup mute button state")
		} else if expected != nodeInfo.Checked {
			return errors.Errorf("UI mute state (%q) does not match audio mute (%q)", nodeInfo.Checked, expected)
		}
		return nil
	}

	// Check UI reflects not muted.
	if err := muteChecked(boolAsChecked(unmutedState)); err != nil {
		return errors.Wrap(err, "failed to confirm  mute is off")
	}
	testing.ContextLog(ctx, "Confirmed mute is off")

	toggleMute := func(from, to checked.Checked) error {
		if err := uiauto.Combine("Toggle mute",
			ui.WaitUntilExists(oss.OSAudioSettingsOutputMuteButton.Attribute("checked", from)),
			ui.LeftClick(oss.OSAudioSettingsOutputMuteButton),
			ui.WaitUntilExists(oss.OSAudioSettingsOutputMuteButton.Attribute("checked", to)),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to click output mute button from (%q) to (%q)", from, to)
		}
		return nil
	}

	// Toggle mute "on".
	testing.ContextLog(ctx, "Toggling mute on")
	if err := toggleMute(checked.False, checked.True); err != nil {
		return errors.Wrap(err, "failed to click output mute button")
	}

	// Get latest mute state.
	muted, err := vh.IsMuted(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to lookup audio mute state")
	}
	if muted == unmutedState {
		return errors.New("Mute state failed to update")
	}

	// Check UI reflects muted.
	if err := muteChecked(boolAsChecked(muted)); err != nil {
		return errors.Wrap(err, "failed to confirm  mute is on")
	}
	testing.ContextLog(ctx, "Confirmed mute toggled on")

	// Toggle mute "off"
	testing.ContextLog(ctx, "Toggling mute off")
	if err := toggleMute(checked.True, checked.False); err != nil {
		return errors.Wrap(err, "failed to click output mute button")
	}

	// Get latest mute state.
	muted, err = vh.IsMuted(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to lookup audio mute state")
	}
	if muted != unmutedState {
		return errors.New("Mute state failed to reset")
	}
	testing.ContextLog(ctx, "Confirmed mute toggled off")

	return nil
}
