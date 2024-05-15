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
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
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

var crasStreamMixUnstableModels = []string{}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasStreamMix,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:         []string{"group:mainline", "group:audio"},
		Desc:         "Captures output audio via loopback and verifies that CRAS plays multiple streams correctly",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "judyhsiao@chromium.org", "yuhsuan@chromium.org"},
		BugComponent: "b:776546",
		// b/291180821: Remove no_qemu after making the test pass on betty.
		SoftwareDeps: []string{"chrome", "no_qemu"},
		Fixture: fixture.AloopLoaded{
			Channels: 2,
			Parent:   fixture.Chrome(),
		}.Instance(),
		Timeout: 3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "rate",
				Val: crasStreamMixVal{
					rate:      44100,
					channel:   2,
					blockSize: 8192,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crasStreamMixUnstableModels...)),
			},
			{
				Name: "blocksize",
				Val: crasStreamMixVal{
					rate:      48000,
					channel:   2,
					blockSize: 512,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crasStreamMixUnstableModels...)),
			},
			{
				Name: "channel",
				Val: crasStreamMixVal{
					rate:      48000,
					channel:   1,
					blockSize: 8192,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crasStreamMixUnstableModels...)),
			},
			{
				Name: "all",
				Val: crasStreamMixVal{
					rate:      44100,
					channel:   1,
					blockSize: 512,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crasStreamMixUnstableModels...)),
			},
			{
				Name:              "all_unstable",
				ExtraAttr:         []string{"informational"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crasStreamMixUnstableModels...)),
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
		rate                 = 48000
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

	if err := audio.SetupLoopback(ctx, cr, s.OutDir(), s.HasError); err != nil {
		s.Fatal("Failed to setup loopback device: ", err)
	}

	param := s.Param().(crasStreamMixVal)

	s.Log("Start Playback")
	// Playback goldenFrequency with different param
	playback1 := testexec.CommandContext(
		ctx, "sox",
		"-b", "16",
		"-r", strconv.Itoa(rate),
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

	filename := fmt.Sprintf("%d_%d_%d", param.rate, param.channel, param.blockSize)
	recording := audio.TestRawData{
		Path:          filepath.Join(s.OutDir(), filename),
		BitsPerSample: 16,
		Channels:      2,
		Rate:          rate,
		Duration:      captureDuration,
	}

	testing.ContextLog(ctx, "Capture output to ", recording.Path+".wav")
	// Capture to arecord to bypass all processing in CRAS.
	if err := testexec.CommandContext(ctx,
		"arecord",
		"-Dhw:Loopback,1",
		fmt.Sprintf("--channels=%d", recording.Channels),
		fmt.Sprintf("--duration=%d", recording.Duration),
		"--format=S16_LE",
		recording.Path+".wav",
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal(err, "failed to capture data")
	}
	testing.ContextLog(ctx, "Convert output to ", recording.Path+".raw")
	// Read file
	// TODO(b/298468964): Deal with wav directly.
	if err := testexec.CommandContext(ctx,
		"sox",
		recording.Path+".wav",
		fmt.Sprintf("--rate=%d", recording.Rate),
		recording.Path+".raw",
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal(err, "failed to convert to RAW")
	}

	tone, err := audio.ReadS16LEPCM(recording.Path+".raw", recording.Channels)
	if err != nil {
		s.Fatal(err, "failed to read recording from file")
	}

	for channel := 0; channel < 2; channel++ {
		if err := audio.CheckFrequency(ctx, tone[channel], float64(recording.Rate), float64(goldenFrequency), 10, incorrectLimit); err != nil {
			s.Errorf("channel %d failed: %v", channel+1, err)
		}
	}
}
