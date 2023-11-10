// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	audiofixture "go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BrowserPlay,
		Desc:         "Tests basic audio playback on ash chrome browser",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "bailideng@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline", "group:audio", "informational"},
		SoftwareDeps: []string{"chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture: audiofixture.AloopLoaded{Channels: 2, Parent: audiofixture.Chrome(
			chrome.ExtraArgs(
				// Chrome requires this to autoplay without user interaction
				// See https://developer.chrome.com/blog/autoplay/
				"--autoplay-policy=no-user-gesture-required",
			),
		)}.Instance(),
		Data: []string{"sine_2ch_440hz_10s_20231101.wav", "audio_playback_test.html"},
	})
}

func BrowserPlay(ctx context.Context, s *testing.State) {
	const (
		cleanupTime          = 45 * time.Second
		captureDuration      = 2 * time.Second
		goldenFrequency      = 440 // Hz
		incorrectLimit       = 3
		rate                 = 48000
		playbackFileChannels = 2
	)
	// Reserve time to remove input file and unload ALSA loopback at the end of the test.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	defer func(ctx context.Context) {
		if err := crastestclient.WaitForNoStream(ctx, 15*time.Second); err != nil {
			// There are still active stream, mark as error and dump audio diagnostic to see the stream info.
			s.Error("Wait for no stream error: ", err)
			if err := crastestclient.DumpAudioDiagnostics(ctx, s.OutDir()); err != nil {
				s.Error("Failed to dump audio diagnostics: ", err)
			}
		}
	}(cleanupCtx)

	// Select loopback device.
	if err := audio.SetupLoopback(ctx, cr); err != nil {
		s.Fatal("Failed to setup loopback device: ", err)
	}

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	conn, err := cr.Browser().NewConn(ctx, server.URL+"/audio_playback_test.html")
	if err != nil {
		s.Fatal("Failed to open new tab: ", err)
	}
	defer conn.Close()

	if err := conn.Eval(ctx, "audio.load()", nil); err != nil {
		s.Fatal("Failed to load audio: ", err)
	}

	if err := conn.Eval(ctx, "audio.play()", nil); err != nil {
		s.Fatal("Failed to start playing: ", err)
	}

	// Run capture.
	recording := audio.TestRawData{
		Path:          filepath.Join(s.OutDir(), "capture.raw"),
		BitsPerSample: 16,
		Channels:      2,
		Rate:          rate,
		Duration:      int(captureDuration.Seconds()),
	}

	testing.ContextLog(ctx, "Capture output to ", recording.Path)
	// Capture to arecord to bypass all processing in CRAS.
	if err := testexec.CommandContext(ctx,
		"arecord",
		"-traw",
		"-Dhw:Loopback,1",
		fmt.Sprintf("--channels=%d", recording.Channels),
		fmt.Sprintf("--rate=%d", recording.Rate),
		fmt.Sprintf("--duration=%d", recording.Duration),
		"--format=S16_LE",
		recording.Path,
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal(err, "failed to capture data")
	}

	if err := conn.WaitForExpr(ctx, "audio.ended"); err != nil {
		s.Fatal("Failed to wait for audio to finish playing: ", err)
	}

	// Verify the correctness of the played audio.
	tone, err := audio.ReadS16LEPCM(recording.Path, recording.Channels)

	if err != nil {
		s.Fatal("Failed to read recording from file: ", err)
	}

	for channel := 0; channel < playbackFileChannels; channel++ {
		if err := audio.CheckFrequency(ctx, tone[channel], float64(rate), float64(goldenFrequency), 10, incorrectLimit); err != nil {
			s.Errorf("Channel %d frequency check failed: %v", channel+1, err)
		}
	}
}
