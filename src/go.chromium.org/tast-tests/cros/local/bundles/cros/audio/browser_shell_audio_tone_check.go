// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/audionode"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BrowserShellAudioToneCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies system tones in browser shell",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:776546",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:intel-nda"},
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		Fixture:      "chromeLoggedIn",
	})
}

func BrowserShellAudioToneCheck(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}
	defer crastestclient.Unmute(ctx)

	vk, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer vk.Close(ctx)

	const expectedAudioNode = "INTERNAL_SPEAKER"
	audioDeviceName, err := audionode.SetAudioNode(ctx, expectedAudioNode)
	if err != nil {
		s.Fatal("Failed to set the Audio node: ", err)
	}

	// Launching browser shell.
	if err := vk.Accel(ctx, "ctrl+alt+t"); err != nil {
		s.Fatal("Failed to press Ctrl+Alt+t to launch browser shell: ", err)
	}

	ui := uiauto.New(tconn)
	// Close browser shell window as cleanup.
	defer func(ctx context.Context) {
		if err := vk.Accel(ctx, "Ctrl+Shift+W"); err != nil {
			s.Fatal("Failed to press Ctrl+Shift+W to close browser shell: ", err)
		}
	}(cleanupCtx)

	croshWindow := nodewith.Name("crosh").Role(role.Window).First()
	if err := ui.WaitForLocation(croshWindow)(ctx); err != nil {
		s.Fatal("Failed to launch browser shell: ", err)
	}

	// Enter 'shell' text and go into chronos@localhost terminal.
	typeSequenceString := []string{"s", "h", "e", "l", "l"}
	if err := vk.TypeSequence(ctx, typeSequenceString); err != nil {
		s.Fatal("Failed to type 'shell' text: ", err)
	}

	if err := vk.Accel(ctx, "enter"); err != nil {
		s.Fatal("Failed to press Enter key: ", err)
	}

	// Load Terminal Settings page.
	if err := vk.Accel(ctx, "Ctrl+Shift+P"); err != nil {
		s.Fatal("Failed to press Ctrl+Shift+P to load Terminal Settings page: ", err)
	}

	behaviorBtton := nodewith.Name("Behavior").Role(role.Link)
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(behaviorBtton)(ctx); err != nil {
		s.Fatal("Failed to find 'Behavior' button: ", err)
	}
	if err := ui.LeftClick(behaviorBtton)(ctx); err != nil {
		s.Fatal("Failed to click 'Behavior' button: ", err)
	}

	// Enable audible terminal bell.
	terminalBell := nodewith.Role(role.CheckBox).Ancestor(nodewith.Name("Audible terminal bell."))
	bellCheckboxInfo, err := ui.Info(ctx, terminalBell)
	if err != nil {
		s.Fatal("Failed to get 'Audible terminal bell' info: ", err)
	}

	if !bellCheckboxInfo.Selected {
		if err := ui.LeftClick(terminalBell)(ctx); err != nil {
			s.Fatal("Failed to click 'Audible terminal bell' button: ", err)
		}
	}

	// Close loaded Terminal Settings page.
	if err := vk.Accel(ctx, "Ctrl+Shift+W"); err != nil {
		s.Fatal("Failed to press Ctrl+Shift+W to close Terminal Settings page: ", err)
	}

	// Press downward arror keyboard key.
	if err := vk.Accel(ctx, "down"); err != nil {
		s.Fatal("Failed to press 'down' key in browser shell: ", err)
	}

	// Bell sound should be emitted by DUT.
	devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
	if err != nil {
		s.Fatal("Failed to detect running output device: ", err)
	}

	if audioDeviceName != devName {
		s.Fatalf("Failed to route the audio tone through expected audio node: got %q; want %q", devName, audioDeviceName)
	}
}
