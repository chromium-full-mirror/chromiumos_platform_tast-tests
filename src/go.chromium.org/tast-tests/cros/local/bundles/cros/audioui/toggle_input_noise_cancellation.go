// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	oss "go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ToggleInputNoiseCancellation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "OS Settings SWA launches to audio subpage  and verifies changing noise cancellation in UI reflected in CRAS",
		// ChromeOS > Software > System Services > Peripherals > Audio (1321112)
		BugComponent: "b:1321112",
		Contacts: []string{
			"cros-peripherals@google.com",
			"ashleydp@google.com",
			"zentaro@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.Microphone(),
			// TODO(b/277583823): Replace model list when a hardware dependency that
			// can check if noise cancellation is supported is available.
			hwdep.Model("anahera", "gimble", "redrix", "yaviks", "yavikso")),
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-816eefa8-76ad-43ec-8300-c747f4b59987",
			},
		},
	})
}

// ToggleInputNoiseCancellation verifies changing input mute in UI reflected in CRAS.
func ToggleInputNoiseCancellation(ctx context.Context, s *testing.State) {
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
	defer func(ctx context.Context) {
		faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

		// Ensure quick settings closed at the end of the test.
		quicksettings.Hide(ctx, tconn)
	}(cleanupCtx)

	if err := oss.LaunchOsSettingsAudioPageFromQuickSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open OS Settings audio page from Quick Settings: ", err)
	}

	s.Log("OS Settings opened to audio settings page")

	if err := verifyNoiseCancellationChanged(ctx, tconn); err != nil {
		s.Fatal("Failed to verify noise cancellation behavior: ", err)
	}
	s.Log("Verified audio noise cancellation behavior")
}

func getQuickSettingsNoiseCancellationInfo(ctx context.Context, tconn *chrome.TestConn) (*uiauto.NodeInfo, error) {
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to open Quick Settings")
	}
	if err := quicksettings.OpenAudioSettings(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to open quick settings")
	}
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)
	qsToggle := nodewith.ClassName("TrayToggleButton").Role(role.Switch).NameContaining("Noise cancellation").Ancestor(nodewith.ClassName("AudioDetailedView"))
	if err := ui.WaitUntilExists(qsToggle)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to focus on noise cancellation toggle")
	}

	return ui.Info(ctx, qsToggle)
}

func isQuickSettingNoiseCancellationEnabled(ctx context.Context, tconn *chrome.TestConn) (checked.Checked, error) {
	nodeInfo, err := getQuickSettingsNoiseCancellationInfo(ctx, tconn)
	if err != nil {
		return checked.Mixed, errors.Wrap(err, "failed to find noise cancellation toggle in Quick Settings")
	}
	return nodeInfo.Checked, nil
}

// verifyNoiseCancellationChanged confirms that changing noise cancellation toggle in UI also updates CRAS.
func verifyNoiseCancellationChanged(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(3 * time.Second)

	ncToggle := nodewith.Role(role.ToggleButton).NameContaining("Noise cancellation")
	const (
		// internalMicLabel is the label used to select an INTERNAL_MIC audio device.
		internalMicLabel string = "Microphone (internal)"

		// internalSpeakerLabel is the label used to select an INTERNAL_SPEAKER audio device.
		internalSpeakerLabel string = "Speaker (internal)"

		// headphonesLabel is the label used to select an HEADPHONES audio device.
		headphonesLabel string = "Headphones"
	)

	if err := quicksettings.SelectAudioOption(ctx, tconn, headphonesLabel); err != nil {
		return errors.Wrapf(err, "failed to set %q as active audio option", headphonesLabel)
	}
	if err := ui.WaitUntilGone(ncToggle)(ctx); err != nil {
		return errors.Wrap(err, "failed to hide noise cancellation when not supported")
	}
	testing.ContextLog(ctx, "Hide noise cancellation hidden when not supported")

	// Select INTERNAL_MIC and INTERNAL_SPEAKER as active nodes to enable noise cancellation.
	if err := quicksettings.SelectAudioOption(ctx, tconn, internalMicLabel); err != nil {
		return errors.Wrapf(err, "failed to set %q as active audio option", internalMicLabel)
	}
	if err := quicksettings.SelectAudioOption(ctx, tconn, internalSpeakerLabel); err != nil {
		return errors.Wrapf(err, "failed to set %q as active audio option", internalSpeakerLabel)
	}
	testing.ContextLog(ctx, "Ensured internal mic and speaker active for noise cancellation")

	ncChecked := func(expected checked.Checked) error {
		nodeInfo, err := ui.Info(ctx, ncToggle)
		if err != nil {
			return errors.Wrapf(err, "failed to locate %p", ncToggle)
		}
		if nodeInfo.Checked != expected {
			return errors.Errorf("failed match noise cancellation toggle state: Expected (%q) Found: (%q)", expected, nodeInfo.Checked)
		}
		return nil
	}

	// Verify Quick Setting and Audio setting page match
	qsMatches := func() error {
		qsToggleChecked, err := isQuickSettingNoiseCancellationEnabled(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to lookup noise cancellation in quick settings")
		}
		if err := ncChecked(qsToggleChecked); err != nil {
			return errors.Wrap(err, "failed to verify noise cancellation match in Quick and Audio setting")
		}
		testing.ContextLog(ctx, "Verified Quick and Audio settings match")
		return nil
	}

	// Find toggle and verify it is off
	if err := ncChecked(checked.False); err != nil {
		return errors.Wrap(err, "failed to verify noise cancellation off")
	}
	if err := qsMatches(); err != nil {
		return errors.Wrap(err, "failed to match quick settings, expected toggle to be off")
	}
	testing.ContextLog(ctx, "Verified noise cancellation off")

	toggleNoiseCancellation := func(state checked.Checked) error {
		if err := uiauto.Combine("Press noise cancellation toggle",
			ui.EnsureFocused(ncToggle),
			ui.DoDefault(ncToggle),
			ui.WaitUntilExists(ncToggle.Attribute("checked", state)))(ctx); err != nil {
			return errors.Wrap(err, "failed to press noise cancellation toggle")
		}
		return nil
	}

	// Press toggle and verify it is on
	if err := toggleNoiseCancellation(checked.True); err != nil {
		return errors.Wrap(err, "failed to toggle noise cancellation on")
	}
	if err := ncChecked(checked.True); err != nil {
		return errors.Wrap(err, "failed to verify noise cancellation on")
	}
	if err := qsMatches(); err != nil {
		return errors.Wrap(err, "failed to match quick settings, expected toggle to be on")
	}
	testing.ContextLog(ctx, "Verified noise cancellation on")

	// Press toggle and verify it is off
	if err := toggleNoiseCancellation(checked.False); err != nil {
		return errors.Wrap(err, "failed to toggle noise cancellation off")
	}
	if err := ncChecked(checked.False); err != nil {
		return errors.Wrap(err, "failed to verify noise cancellation off")
	}
	if err := qsMatches(); err != nil {
		return errors.Wrap(err, "failed to match quick settings, expected toggle to be off")
	}
	testing.ContextLog(ctx, "Verified noise cancellation off")

	return nil
}
