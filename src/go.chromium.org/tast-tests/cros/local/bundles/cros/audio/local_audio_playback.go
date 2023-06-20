// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalAudioPlayback,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Play local audio file through default app and check if the audio is routing through expected device",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "bailideng@google.com"},
		BugComponent: "b:776546",
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		Params: []testing.Param{{
			Name:      "internal_speaker",
			ExtraAttr: []string{"group:mainline", "informational", "group:intel-gating", "group:intel-nda"},
			Val:       "Speaker (internal)",
		}, {
			Name:      "headphone",
			Val:       "Headphone",
			ExtraAttr: []string{"group:intel-jack"},
		}, {
			Name:      "usb_speaker",
			Val:       "USB",
			ExtraAttr: []string{"group:intel-usb-set2"},
		}},
	})
}

// LocalAudioPlayback generates audio file and plays it through default audio player.
// Switching nodes via UI interactions is the recommended way, instead of using
// cras.SetActiveNode() method, as UI will always send the preference input/output
// devices to CRAS. Calling cras.SetActiveNode() changes the active devices for a
// moment, but they soon are reverted by UI. See (b/191602192) for details.
func LocalAudioPlayback(ctx context.Context, s *testing.State) {
	expectedOutputDevice := s.Param().(string)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	timeForCleanup := 10 * time.Second
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, timeForCleanup)
	defer cancel()

	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}
	defer crastestclient.Unmute(cleanupCtx)

	// Generate sine raw input file that lasts 30 seconds.
	rawFileName := "30SEC.raw"
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	rawFilePath := filepath.Join(downloadsPath, rawFileName)
	rawFile := audio.TestRawData{
		Path:          rawFilePath,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Frequencies:   []int{440, 440},
		Volume:        0.05,
		Duration:      30,
	}
	if err := audio.GenerateTestRawData(ctx, rawFile); err != nil {
		s.Fatal("Failed to generate audio test data: ", err)
	}
	defer os.Remove(rawFile.Path)

	wavFileName := "30SEC.wav"
	wavFile := filepath.Join(downloadsPath, wavFileName)
	if err := audio.ConvertRawToWav(ctx, rawFile, wavFile); err != nil {
		s.Fatal("Failed to convert raw to wav: ", err)
	}
	defer os.Remove(wavFile)

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	// Open the test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}
	defer files.Close(cleanupCtx)
	if err := files.OpenDownloads()(ctx); err != nil {
		s.Fatal("Failed to open Downloads folder in files app: ", err)
	}
	// Open the audio file.
	// The audio file automatically plays once it is opened in the gallery app.
	if err := files.OpenFile(wavFileName)(ctx); err != nil {
		s.Fatalf("Failed to open the audio file %q: %v", wavFileName, err)
	}
	// Closing the audio player.
	defer func() {
		if kb.Accel(cleanupCtx, "Ctrl+W"); err != nil {
			s.Error("Failed to close Audio player: ", err)
		}
	}()

	// Select output device.
	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show Quick Settings")
	}
	defer quicksettings.Hide(cleanupCtx, tconn)

	if err := quicksettings.SelectAudioOption(ctx, tconn, expectedOutputDevice); err != nil {
		s.Fatal("Failed to select audio option: ", err)
	}

	// Get Current active node.
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object")
	}
	audioDeviceName, _, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get the selected audio device: ", err)
	}

	// Check that there is running audio stream, and the used device matches the
	// selected output device.
	devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
	if err != nil {
		s.Fatal("Failed to detect running output device: ", err)
	}

	if audioDeviceName != devName {
		s.Fatalf("Failed to route the audio through expected audio node: got %q; want %q", devName, audioDeviceName)
	}
}
