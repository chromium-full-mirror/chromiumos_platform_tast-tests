// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Test is expected to end in 17 Seconds = 7 seconds of testing + 10 seconds of cleanup
// Timeline:
// --- time -->: 01234567
//      capture: --ccccc
//     playback: ppp----

const crasPlaybackRecordTimeout = 17 * time.Second * 2

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasPlaybackRecord,
		Desc:         "Verifies the device can record and playback at the same time correctly with CRAS",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "bailideng@google.com"},
		BugComponent: "b:776546",
		HardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Speaker()),
		Attr: []string{
			"group:mainline",
			"informational",
		},
		VariantCategory: `{"name": "Audio_SoC_Codec_Amp"}`,
		Timeout:         crasPlaybackRecordTimeout,
		Fixture:         "rebootForAudioDSPFixture",
	})
}

func CrasPlaybackRecord(ctx context.Context, s *testing.State) {
	const (
		recordDuration   = 5 * time.Second
		playbackDuration = 3 * time.Second
		sleepDuration    = 2 * time.Second
	)

	// Dump cras info when test failed.
	defer crastestclient.DumpAudioDiagnosticsOnError(ctx, s.OutDir(), s.HasError)

	// Use a shorter context to save time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to cras: ", err)
	}

	outputDev, err := cras.GetNodeByMatcher(ctx, nodematch.Type("INTERNAL_SPEAKER"))
	if err != nil {
		s.Fatal("Failed to get internal speaker node from cras: ", err)
	}

	inputDev, err := cras.GetNodeByMatcher(ctx, nodematch.Type("INTERNAL_MIC"))
	if err != nil {
		s.Fatal("Failed to get internal mic node from cras: ", err)
	}

	go func() {
		testing.ContextLogf(ctx, "Playing audio from %s", outputDev.DeviceName)
		playbackContext, cancel := context.WithTimeout(ctx, playbackDuration+time.Second)
		defer cancel()
		if err := testexec.CommandContext(
			playbackContext, "cras_test_client",
			"--duration", strconv.Itoa(int(playbackDuration.Seconds())),
			"-c", "2",
			"-f", "S16_LE",
			"-r", "48000",
			"--playback_file", "/dev/zero").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to play audio with Cras: ", err)
		}
	}()

	// GoBigSleepLint: Wait sleep_duration for playback command to play
	testing.Sleep(ctx, sleepDuration)

	testing.ContextLogf(ctx, "Capturing audio from %s", inputDev.DeviceName)

	recordContext, cancel := context.WithTimeout(ctx, recordDuration+time.Second)
	defer cancel()
	if err := testexec.CommandContext(
		recordContext, "cras_test_client",
		"--duration", strconv.Itoa(int(recordDuration.Seconds())),
		"-c", "2",
		"-f", "S16_LE",
		"-r", "48000",
		"--capture_file", "/dev/null").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to record audio with ALSA: ", err)
	}
}
