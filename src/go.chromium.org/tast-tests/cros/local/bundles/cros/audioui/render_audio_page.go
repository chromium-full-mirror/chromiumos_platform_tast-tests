// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
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

	if err := verifyLayoutComponents(ctx, tconn); err != nil {
		s.Fatal("Failed to verify audio setting page layout: ", err)
	}
	s.Log("Verified audio settings layout")
}

// verifyLayoutComponents tests if audio input and output sections are displayed.
func verifyLayoutComponents(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	for _, node := range []*nodewith.Finder{
		oss.OSAudioSettingsOutputDeviceDropdown,
		oss.OSAudioSettingsOutputMuteButton,
		oss.OSAudioSettingsOutputVolumeSlider,
		oss.OSAudioSettingsInputDeviceDropdown,
		oss.OSAudioSettingsInputMuteButton,
		oss.OSAudioSettingsInputVolumeSlider,
	} {
		nodeInfo, err := ui.Info(ctx, node)
		if err != nil {
			return errors.Wrap(err, "failed to get node info")
		}
		testing.ContextLogf(ctx, "Found %q %q", nodeInfo.Name, nodeInfo.ClassName)
	}
	return nil
}
