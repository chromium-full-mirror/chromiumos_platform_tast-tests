// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type crasTimestampInOrderParameters struct {
	node     string
	argument string
}

var unstableModelsTimestampInOrder = []string{
	// TODO(b/333498840): Undo skip after fix.
	"treeya", "liara", "kasumi360",
	// TODO(b/333498998): Undo skip after fix.
	"craaskbowl", "hideo",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasTimestampInOrder,
		Desc:         "Verifies CRAS client receives timestamps that are in order",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "bailideng@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "rebootForAudioDSPFixture",
		Params: []testing.Param{{
			Name:              "capture",
			ExtraHardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.SkipOnModel(unstableModelsTimestampInOrder...)),
			Val: crasTimestampInOrderParameters{
				node:     "INTERNAL_MIC",
				argument: "--capture_file=/dev/null",
			},
		},
			{
				Name:              "capture_unstable",
				ExtraHardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Model(unstableModelsTimestampInOrder...)),
				Val: crasTimestampInOrderParameters{
					node:     "INTERNAL_MIC",
					argument: "--capture_file=/dev/null",
				},
			}, {
				Name:              "playback",
				ExtraHardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.SkipOnModel(unstableModelsTimestampInOrder...)),
				Val: crasTimestampInOrderParameters{
					node:     "INTERNAL_SPEAKER",
					argument: "--playback_file=/dev/zero",
				},
			}, {
				Name:              "playback_unstable",
				ExtraHardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.Model(unstableModelsTimestampInOrder...)),
				Val: crasTimestampInOrderParameters{
					node:     "INTERNAL_SPEAKER",
					argument: "--playback_file=/dev/zero",
				},
			}},
	})
}

func CrasTimestampInOrder(ctx context.Context, s *testing.State) {
	const duration = 10 * time.Second
	param := s.Param().(crasTimestampInOrderParameters)

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

	if err := cras.SetActiveNodeByType(ctx, param.node); err != nil {
		s.Fatalf("Failed to set %s active: %s", param.node, err)
	}

	// Set timeout to duration + 5s, which is the time buffer to complete the normal execution.
	runCtx, cancel := context.WithTimeout(ctx, duration+5*time.Second)
	defer cancel()

	// Run function by CRAS.
	if err := testexec.CommandContext(
		runCtx, "cras_test_client",
		param.argument,
		"--duration", strconv.Itoa(int(duration.Seconds())),
		"--num_channels", "2",
		"--rate", "48000",
		"--show_ooo_timestamp",
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to obtain in-order timestamps: ", err)
	}
}
