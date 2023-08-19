// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type crasStreamMixVal struct {
	rate      int
	channel   int
	blockSize int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasStreamMix,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:         []string{"group:mainline", "group:audio"},
		Desc:         "Captures output audio via loopback and verifies that CRAS plays multiple streams correctly",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "judyhsiao@chromium.org", "yuhsuan@chromium.org"},
		BugComponent: "b:776546",
		// b/291180821: Remove no_qemu after making the test pass on betty.
		SoftwareDeps: []string{"audio_stable", "chrome", "no_qemu"},
		// b/291180821: Skip "kevin", "wormdingler", "quackingstick", "homestar", "pazquel", "limozeen", "pompom", "kingoftown", "pazquel360" until b/291180821 is fixed.
		// b/293263346: Skip "gladios", "lisbon" before b/293263346 is fixed.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("kevin", "wormdingler", "quackingstick", "homestar", "pazquel", "limozeen", "pompom", "kingoftown", "pazquel360", "gladios", "lisbon")),
		Pre:          chrome.LoggedIn(),
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "rate",
				Val: crasStreamMixVal{
					rate:      44100,
					channel:   2,
					blockSize: 8192,
				},
			},
			{
				Name:      "blocksize",
				ExtraAttr: []string{"informational", "group:criticalstaging"},
				Val: crasStreamMixVal{
					rate:      48000,
					channel:   2,
					blockSize: 512,
				},
			},
			{
				Name: "channel",
				Val: crasStreamMixVal{
					rate:      48000,
					channel:   1,
					blockSize: 8192,
				},
			},
			{
				Name:      "all",
				ExtraAttr: []string{"informational", "group:criticalstaging"},
				Val: crasStreamMixVal{
					rate:      44100,
					channel:   1,
					blockSize: 512,
				},
			},
		},
	})
}

func CrasStreamMix(ctx context.Context, s *testing.State) {
	const (
		cleanupTime          = 45 * time.Second
		captureDuration      = 2 // second(s)
		playbackDuration     = 6 // second(s)
		waitForStreamTimeout = 2 * time.Second
		goldenFrequency      = 440 // Hz
		incorrectLimit       = 3
	)

	// Reserve time to remove input file and unload ALSA loopback at the end of the test.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	cr := s.PreValue().(*chrome.Chrome)
	// Load ALSA loopback module.
	unload, err := audio.LoadAloop(ctx)
	if err != nil {
		s.Fatal("Failed to load ALSA loopback module: ", err)
	}
	defer func(ctx context.Context) {
		if err := crastestclient.WaitForNoStream(ctx, 15*time.Second); err != nil {
			// There are still active stream, mark as error and dump audio diagnostic to see the stream info.
			s.Error("Wait for no stream error: ", err)
			if err := crastestclient.DumpAudioDiagnostics(ctx, s.OutDir()); err != nil {
				s.Error("Failed to dump audio diagnostics: ", err)
			}
		}
		unload(ctx)
	}(cleanupCtx)

	if err = audio.SetupLoopback(ctx, cr); err != nil {
		s.Fatal("Failed to setup loopback device: ", err)
	}

	param := s.Param().(crasStreamMixVal)

	s.Log("Start Playback")
	// Playback goldenFrequency with different param
	playback1 := testexec.CommandContext(
		ctx, "sox",
		"-b", "16",
		"-r", "48000",
		"-c", "2",
		"--buffer", strconv.Itoa(8192),
		"-n",
		"-t",
		"alsa",
		"default",
		"synth", strconv.Itoa(playbackDuration),
		"sine", strconv.Itoa(goldenFrequency))

	// Playback zeros with different param.
	playback2 := testexec.CommandContext(
		ctx, "sox",
		"-b", "16",
		"-r", strconv.Itoa(param.rate),
		"-c", strconv.Itoa(param.channel),
		"--buffer", strconv.Itoa(param.blockSize),
		"-n",
		"-t",
		"alsa",
		"default",
		"synth", strconv.Itoa(playbackDuration),
		"sine", strconv.Itoa(0))

	playback1.Start()
	playback2.Start()

	if _, err := crastestclient.WaitForStreams(ctx, waitForStreamTimeout); err != nil {
		s.Fatal(err, "failed to playback within timeout")
	}

	filename := fmt.Sprintf("%d_%d_%d.raw", param.rate, param.channel, param.blockSize)
	recording := audio.TestRawData{
		Path:          filepath.Join(s.OutDir(), filename),
		BitsPerSample: 16,
		Channels:      8, // Loopback module has 8 channels.
		Rate:          48000,
		Duration:      captureDuration,
	}

	testing.ContextLog(ctx, "Capture output to ", recording.Path)
	if err := crastestclient.CaptureFileCommand(
		ctx, recording.Path,
		recording.Duration,
		recording.Channels,
		recording.Rate).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal(err, "failed to capture")
	}

	tone, err := audio.ReadS16LEPCM(recording.Path, recording.Channels)
	if err != nil {
		s.Fatal(err, "failed to read recording from file")
	}

	for channel := 0; channel < 2; channel++ {
		if err := audio.CheckFrequency(ctx, tone[channel], float64(recording.Rate), float64(goldenFrequency), 10, incorrectLimit); err != nil {
			s.Errorf("channel %d failed: %v", channel+1, err)
		}
	}
}
