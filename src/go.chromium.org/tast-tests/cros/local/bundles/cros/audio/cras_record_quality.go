// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// TODO(b/244254621) : remove "sasukette" when b/244254621 is fixed.
// TODO(b/256522810) : remove "steelix" when b/256522810 is fixed.
// TODO(b/256523120) : remove "gimble" when b/256523120 is fixed.
// TODO(b/312097873) : remove "brya" when b/309904720 is fixed.
var crasRecordQualityUnstableModels = []string{"sasukette", "steelix", "gimble", "brya"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasRecordQuality,
		Desc:         "Verifies recorded samples from CRAS are correct",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "yuhsuan@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:776546",
		HardwareDeps: hwdep.D(hwdep.Microphone()),
		Attr:         []string{"group:mainline"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crasRecordQualityUnstableModels...)),
		}, {
			Name:              "unstable_model",
			ExtraHardwareDeps: hwdep.D(hwdep.Model(crasRecordQualityUnstableModels...)),
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func CrasRecordQuality(ctx context.Context, s *testing.State) {
	const duration = 2 * time.Second

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	// Stop UI in advance for this test to avoid the node being selected by UI.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui: ", err)
	}
	defer upstart.EnsureJobRunning(cleanupCtx, "ui")

	defer func(ctx context.Context) {
		if s.HasError() {
			if err := crastestclient.DumpAudioDiagnostics(ctx, s.OutDir()); err != nil {
				s.Error("Failed to dump audio diagnostics: ", err)
			}
		}
	}(cleanupCtx)

	cras, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}

	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_MIC"); err != nil {
		s.Fatal("Failed to set internal mic active: ", err)
	}

	// Set timeout to duration + 5s, which is the time buffer to complete the normal execution.
	runCtx, cancel := context.WithTimeout(ctx, duration+5*time.Second)
	defer cancel()

	rawFile := filepath.Join(s.OutDir(), "recorded.raw")
	wavFile := filepath.Join(s.OutDir(), "recorded.wav")
	clippedFile := filepath.Join(s.OutDir(), "clipped.wav")

	// Record function by CRAS.
	if err := testexec.CommandContext(
		runCtx, "cras_test_client",
		"--capture_file", rawFile,
		"--duration", strconv.FormatFloat(duration.Seconds(), 'f', -1, 64),
		"--num_channels", "2",
		"--rate", "48000",
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to record from CRAS: ", err)
	}
	rawData := audio.TestRawData{
		Path:          rawFile,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
	}
	if err := audio.ConvertRawToWav(ctx, rawData, wavFile); err != nil {
		s.Fatal("Failed to convert raw to wav: ", err)
	}

	// Remove first 0.5 seconds to avoid pop noise.
	if err := audio.TrimFileFrom(ctx, wavFile, clippedFile, 500*time.Millisecond); err != nil {
		s.Fatal("Failed to convert raw to wav: ", err)
	}

	if err := audio.CheckRecordingNotZero(ctx, clippedFile); err != nil {
		s.Error("Failed to check quality: ", err)
	}
}
