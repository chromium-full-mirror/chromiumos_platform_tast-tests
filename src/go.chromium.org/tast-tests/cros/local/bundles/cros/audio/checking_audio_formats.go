// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"path"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CheckingAudioFormats,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies supported audio file formats",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		BugComponent: "b:776546",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline"},
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		Data:         []string{"audio.flac", "audio.m4a", "audio.ogg", "audio.wav", "audio.mp3", "audio.5.1.mp3"},
		Fixture:      "chromeLoggedInQsRevampEnabled",
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"audio_stable"},
				ExtraAttr:         []string{"group:intel-nda"},
			}, {
				Name:              "unstable_platform",
				ExtraSoftwareDeps: []string{"audio_unstable"},
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func CheckingAudioFormats(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up capture (aloop) module.
	unload, err := audio.LoadAloop(ctx)
	if err != nil {
		s.Fatal("Failed to load ALSA loopback module: ", err)
	}

	defer func(ctx context.Context) {
		// Wait for no stream before unloading aloop as unloading while there is a stream
		// will cause the stream in ARC to be in an invalid state.
		if err := crastestclient.WaitForNoStream(ctx, 5*time.Second); err != nil {
			s.Error("Wait for no stream error: ", err)
		}
		unload(ctx)
	}(cleanupCtx)

	// Select ALSA loopback output and input nodes as active nodes by UI.
	// Call Hide() and Show() to reset the quicksettings menu first.
	quicksettings.Hide(ctx, tconn)
	quicksettings.Show(ctx, tconn)
	if err := quicksettings.SelectAudioOption(ctx, tconn, "Loopback Playback"); err != nil {
		s.Fatal("Failed to select ALSA loopback output: ", err)
	}

	// After selecting Loopback Playback, SelectAudioOption() sometimes detected that audio setting
	// is still opened while it is actually fading out, and failed to select Loopback Capture.
	// Call Hide() and Show() to reset the quicksettings menu first.
	quicksettings.Hide(ctx, tconn)
	quicksettings.Show(ctx, tconn)
	if err := quicksettings.SelectAudioOption(ctx, tconn, "Loopback Capture"); err != nil {
		s.Fatal("Failed to select ALSA loopback input: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	audioFiles := []string{"audio.flac", "audio.m4a", "audio.ogg", "audio.wav", "audio.mp3", "audio.5.1.mp3"}
	audioFileRe := regexp.MustCompile(`^audio.(wav|m4a|ogg|flac|mp3|5.1.mp3)$`)

	for _, file := range audioFiles {
		if err := fsutil.CopyFile(s.DataPath(file), path.Join(downloadsPath, file)); err != nil {
			s.Fatalf("Failed to copy %q file to %q: %v", file, downloadsPath, err)
		}
	}

	defer func(context.Context) {
		for _, file := range audioFiles {
			if err := os.Remove(path.Join(downloadsPath, file)); err != nil {
				s.Fatalf("Failed to remove %q file: %v", file, err)
			}
		}
	}(cleanupCtx)

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard eventwriter: ", err)
	}
	defer kb.Close(ctx)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}

	if err := files.OpenDownloads()(ctx); err != nil {
		s.Fatal("Failed to open Downloads folder in files app: ", err)
	}
	defer files.Close(cleanupCtx)

	for _, file := range audioFiles {
		if !audioFileRe.MatchString(file) {
			s.Fatalf("Unknown audio file format, want %q; got %q", audioFileRe, file)
		}

		if err := files.OpenFile(file)(ctx); err != nil {
			s.Fatalf("Failed to open the audio file %q: %v", file, err)
		}

		// GoBigSleepLint: Sample time for the audio to play for 5 seconds.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Error while waiting during sample time: ", err)
		}

		_, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
		if err != nil {
			s.Fatal("Failed to detect running audio stream: ", err)
		}

		// Closing the audio player.
		if kb.Accel(ctx, "Ctrl+W"); err != nil {
			s.Error("Failed to close Audio player: ", err)
		}
	}
}
