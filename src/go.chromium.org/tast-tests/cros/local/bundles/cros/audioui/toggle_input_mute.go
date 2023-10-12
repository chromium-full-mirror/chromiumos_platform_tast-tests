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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	oss "go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ToggleInputMute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "OS Settings SWA launches to audio subpage  and verifies changing input mute in UI reflected in CRAS",
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

// ToggleInputMute verifies changing input mute in UI reflected in CRAS.
func ToggleInputMute(ctx context.Context, s *testing.State) {
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

	s.Log("OS Settings opened to audio settings page")

	if err := verifyInputMuteChanged(ctx, tconn); err != nil {
		s.Fatal("Failed to verify input mute behavior: ", err)
	}
	s.Log("Verified audio input mute behavior")
}

// verifyInputMuteChanged confirms that changing input mute in UI also updates CRAS.
func verifyInputMuteChanged(ctx context.Context, tconn *chrome.TestConn) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cras, err := audio.NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to setup CRAS")
	}

	// Ensure device un-muted at end of test.
	defer func(ctx context.Context, cras *audio.Cras) {
		if err := cras.SetInputMute(ctx, false); err != nil {
			testing.ContextLog(ctx, "Failed to unmute input: ", err)
		}
	}(cleanupCtx, cras)

	testing.ContextLog(ctx, "Ensure device is not muted")
	if err := cras.SetInputMute(ctx, false); err != nil {
		return errors.Wrap(err, "failed to set input as not muted")
	}

	// Check volume state in CRAS.
	unmutedVolstate, err := cras.GetVolumeState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to lookup audio volume state")
	}
	if unmutedVolstate.InputMute {
		return errors.Wrap(err, "failed to initialize input to not muted")
	}

	ui := uiauto.New(tconn).WithTimeout(2 * time.Second)

	// Check UI reflects not muted.
	boolAsChecked := func(state bool) checked.Checked {
		if state {
			return checked.True
		}
		return checked.False
	}
	muteChecked := func(expected checked.Checked) error {
		if nodeInfo, err := ui.Info(ctx, oss.OSAudioSettingsInputMuteButton); err != nil {
			return errors.Wrap(err, "failed to lookup mute button state")
		} else if expected != nodeInfo.Checked {
			return errors.Errorf("UI input mute state (%q) does not match audio mute (%q)", nodeInfo.Checked, expected)
		}
		return nil
	}
	if err := muteChecked(boolAsChecked(unmutedVolstate.InputMute)); err != nil {
		return errors.Wrap(err, "failed to confirm mute is off")
	}
	testing.ContextLog(ctx, "Confirmed input mute is off")

	// Toggle mute "on".
	toggleMute := func(muted bool) error {
		n := 1
		checked := "false"
		// Input mute button normally the second toggle button in the UI. When
		// toggled on it is the only "checked" button in the UI so the Finder.Nth
		// needs to reflect that.
		if muted {
			checked = "true"
			n = 0
		}
		if err := uiauto.Combine("Toggle input mute",
			ui.LeftClick(oss.OSAudioSettingsInputMuteButton),
			ui.WaitUntilExists(nodewith.NameContaining("Volume").Nth(n).Attribute("checked", checked)),
		)(ctx); err != nil {
			nodeInfo, err2 := ui.Info(ctx, oss.OSAudioSettingsInputMuteButton)
			if err2 != nil {
				testing.ContextLog(ctx, "Cant find node at all")
			}
			testing.ContextLogf(ctx, "NodeInfo: %p", nodeInfo)
			return errors.Wrap(err, "failed to click input mute button")
		}
		return nil
	}

	testing.ContextLog(ctx, "Toggling input mute on")
	if err := toggleMute(true); err != nil {
		return errors.Wrap(err, "failed to toggle mute on")
	}

	// Get latest mute state.
	volstate, err := cras.GetVolumeState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to lookup audio volume state")
	}

	// Check UI reflects muted.
	if err := muteChecked(boolAsChecked(volstate.InputMute)); err != nil {
		return errors.Wrap(err, "failed to confirm mute is on")
	}
	testing.ContextLog(ctx, "Confirmed input mute toggled on")

	// Toggle mute "off"
	testing.ContextLog(ctx, "Toggling input mute off")
	if err := toggleMute(false); err != nil {
		return errors.Wrap(err, "failed to toggle mute off")
	}

	// Get latest mute state.
	volstate, err = cras.GetVolumeState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to lookup audio input mute state")
	}
	if err := muteChecked(boolAsChecked(volstate.InputMute)); err != nil {
		return errors.Wrap(err, "failed to confirm mute is off")
	}
	testing.ContextLog(ctx, "Confirmed input mute toggled off")

	return nil
}
