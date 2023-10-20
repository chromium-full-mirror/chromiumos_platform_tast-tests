// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/device"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasRecordingCorrectness,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:         []string{"group:mainline", "group:audio", "informational"},
		Desc:         "Play audio to loopback by aplay and verifies that CRAS records audio correctly",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "judyhsiao@chromium.org", "yuhsuan@chromium.org"},
		BugComponent: "b:776546",
		// b/291180821: Remove no_qemu after making the test pass on betty.
		SoftwareDeps: []string{"audio_stable", "chrome", "no_qemu"},
		Fixture:      fixture.AloopLoaded{Channels: 2}.Instance(),
		Timeout:      3 * time.Minute,
	})
}

func CrasRecordingCorrectness(ctx context.Context, s *testing.State) {
	const (
		cleanupTime      = 45 * time.Second
		getDeviceTimeout = 5 * time.Second
		captureDuration  = 2 * time.Second
		playbackDuration = 10 * time.Second
		goldenFrequency  = 440 // Hz
		incorrectLimit   = 3
		rate             = 48000
	)

	// Reserve time to remove input file and unload ALSA loopback at the end of the test.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	cr, err := chrome.New(ctx)
	defer cr.Close(cleanupCtx)

	defer func(ctx context.Context) {
		if err := crastestclient.WaitForNoStream(ctx, 15*time.Second); err != nil {
			// There are still active stream, mark as error and dump audio diagnostic to see the stream info.
			s.Error("Wait for no stream error: ", err)
			if err := crastestclient.DumpAudioDiagnostics(ctx, s.OutDir()); err != nil {
				s.Error("Failed to dump audio diagnostics: ", err)
			}
		}
	}(cleanupCtx)

	if err := audio.SetupLoopback(ctx, cr); err != nil {
		s.Fatal("Failed to setup loopback device: ", err)
	}

	noiseWave := filepath.Join(s.OutDir(), "noise.wav")
	if err := testexec.CommandContext(
		ctx,
		"sox",
		"-n", "-L",
		"-e", "signed-integer",
		"-b", "16",
		"-r", strconv.Itoa(rate),
		"-c", "2",
		noiseWave,
		"synth", strconv.FormatFloat(playbackDuration.Seconds(), 'f', -1, 64),
		"sine", strconv.Itoa(goldenFrequency),
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Cannot generate noise.wav: ", err)
	}

	playbackDone := make(chan struct{})
	go func() {
		defer close(playbackDone)
		// Run playback.
		if err := audio.PlayWavToPCM(ctx, noiseWave, device.AloopPlaybackPCM); err != nil {
			s.Error("Cannot run playback: ", err)
		}
	}()

	checkRunningDevice := func(ctx context.Context) error {
		return device.CheckRunningDevice(ctx, true, false)
	}

	if err := testing.Poll(ctx, checkRunningDevice, &testing.PollOptions{Timeout: getDeviceTimeout}); err != nil {
		s.Fatal("Failed to detect running device: ", err)
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
	if err := crastestclient.CaptureFileCommand(
		ctx, recording.Path,
		recording.Duration,
		recording.Channels,
		recording.Rate).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal(err, "failed to capture")
	}

	s.Log("Waiting for playback to complete")
	<-playbackDone

	tone, err := audio.ReadS16LEPCM(recording.Path, 2)

	if err != nil {
		s.Fatal(err, "failed to read recording from file")
	}

	for channel := 0; channel < 2; channel++ {
		if err := audio.CheckFrequency(ctx, tone[channel], float64(rate), float64(goldenFrequency), 10, incorrectLimit); err != nil {
			s.Errorf("channel %d failed: %v", channel+1, err)
		}
	}
}
