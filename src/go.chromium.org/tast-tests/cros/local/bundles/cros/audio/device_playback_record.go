// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Test is expected to end in 17 Seconds = 7 seconds of testing + 10 seconds of cleanup
// Timeline:
// --- time -->: 01234567
//      capture: ---ccccc
//     playback: pppp----

const devicePlaybackRecordTimeout = 17 * time.Second * 2

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevicePlaybackRecord,
		Desc:         "Verifies the device can record and playback at the same time correctly with ALSA commands",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "htcheong@chromium.org"},
		BugComponent: "b:776546",
		HardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Speaker()),
		Attr: []string{
			"group:mainline",
		},
		VariantCategory: `{"name": "Audio_SoC_Codec_Amp"}`,
		Timeout:         devicePlaybackRecordTimeout,
		Fixture:         "rebootForAudioDSPFixture",
	})
}

func DevicePlaybackRecord(ctx context.Context, s *testing.State) {
	const (
		recordDuration   = 5 * time.Second
		playbackDuration = 3 * time.Second
		sleepDuration    = 2 * time.Second
	)

	// Dump alsa info when test failed.
	defer crastestclient.DumpAudioDiagnosticsOnError(ctx, s.OutDir(), s.HasError)

	// Use a shorter context to save time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Stop UI in advance for this test to avoid the node being selected by UI.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui: ", err)
	}
	defer upstart.EnsureJobRunning(ctx, "ui")

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to cras: ", err)
	}

	// Select internal mic and internal speaker explicitly to prevent mixer control issues.
	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_MIC"); err != nil {
		s.Fatal("Failed to set internal mic active: ", err)
	}

	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_SPEAKER"); err != nil {
		s.Fatal("Failed to set internal speaker active: ", err)
	}

	outputDev, err := cras.GetNodeByMatcher(ctx, nodematch.Type("INTERNAL_SPEAKER"))
	if err != nil {
		s.Fatal("Failed to get internal speaker node from cras: ", err)
	}
	outputAlsaPCM := "plughw:" + strings.Split(outputDev.DeviceName, ":")[2]

	inputDev, err := cras.GetNodeByMatcher(ctx, nodematch.Type("INTERNAL_MIC"))
	if err != nil {
		s.Fatal("Failed to get internal mic node from cras: ", err)
	}
	inputAlsaPCM := "plughw:" + strings.Split(inputDev.DeviceName, ":")[2]

	// Stop cras to avoid device getting occupied.
	testing.ContextLog(ctx, "Stopping CRAS")
	if err := upstart.StopJob(ctx, "cras"); err != nil {
		s.Fatal("Failed to stop cras: ", err)
	}
	defer func(ctx context.Context) {
		// Restart CRAS.
		s.Log("Starting CRAS")
		if err := upstart.EnsureJobRunning(ctx, "cras"); err != nil {
			s.Fatal("Failed to start CRAS: ", err)
		}
	}(cleanupCtx)

	go func() {
		testing.ContextLogf(ctx, "Playing audio from %s", outputAlsaPCM)
		playbackContext, cancel := context.WithTimeout(ctx, playbackDuration+time.Second*2)
		defer cancel()
		if err := testexec.CommandContext(
			playbackContext, "aplay",
			"-d", strconv.Itoa(int(playbackDuration.Seconds())),
			"-c", "2",
			"-f", "S16_LE",
			"-r", "48000",
			"-D", outputAlsaPCM,
			"/dev/zero").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to play audio with ALSA: ", err)
		}
	}()

	// GoBigSleepLint: Wait sleep_duration for playback command to play
	testing.Sleep(ctx, sleepDuration)

	testing.ContextLogf(ctx, "Capturing audio from %s", inputAlsaPCM)

	recordContext, cancel := context.WithTimeout(ctx, recordDuration+time.Second*2)
	defer cancel()
	if err := testexec.CommandContext(
		recordContext, "arecord",
		"-d", strconv.Itoa(int(recordDuration.Seconds())),
		"-c", "2",
		"-f", "S16_LE",
		"-r", "48000",
		"-D", inputAlsaPCM,
		"/dev/null").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to record audio with ALSA: ", err)
	}
}
