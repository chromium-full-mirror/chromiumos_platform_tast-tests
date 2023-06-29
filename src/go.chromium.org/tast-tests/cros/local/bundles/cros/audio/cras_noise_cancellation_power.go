// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasNoiseCancellationPower,
		LacrosStatus: testing.LacrosVariantUnneeded, // Ash is only a platform dependency.
		Desc:         "Collect power metrics of using noise cancellation in CRAS",
		BugComponent: "b:776546",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		Fixture:      "powerAshPlatformAudio",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Speaker()),
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Timeout:      10*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{
			{
				Name: "no_effects",
				Val: crasNoiseCancellationPowerParam{
					crasSetUp: func(ctx context.Context, s *testing.State) {
						cras, err := audio.RestartCras(ctx)
						if err != nil {
							s.Fatal("Failed to restart CRAS: ", err)
						}
						if err := cras.SetActiveNodeByType(ctx, "INTERNAL_MIC"); err != nil {
							s.Fatal("Failed to set internal mic active: ", err)
						}
						if err := cras.SetActiveNodeByType(ctx, "INTERNAL_SPEAKER"); err != nil {
							s.Fatal("Failed to set internal speaker active: ", err)
						}
					},
					extraCrasClientArgs: nil,
				},
			},
			{
				Name: "aec",
				Val: crasNoiseCancellationPowerParam{
					crasSetUp: func(ctx context.Context, s *testing.State) {
						cras, err := audio.RestartCras(ctx)
						if err != nil {
							s.Fatal("Failed to restart CRAS: ", err)
						}
						if err := cras.SetActiveNodeByType(ctx, "INTERNAL_MIC"); err != nil {
							s.Fatal("Failed to set internal mic active: ", err)
						}
						if err := cras.SetActiveNodeByType(ctx, "INTERNAL_SPEAKER"); err != nil {
							s.Fatal("Failed to set internal speaker active: ", err)
						}
						if err := cras.SetNoiseCancellationEnabled(ctx, false); err != nil {
							s.Fatal("Failed to SetNoiseCancellationEnabled: ", err)
						}
					},
					extraCrasClientArgs: []string{"--effects=aec"},
				},
			},
			{
				Name: "aec_nc",
				Val: crasNoiseCancellationPowerParam{
					crasSetUp: func(ctx context.Context, s *testing.State) {
						cras, err := audio.RestartCras(ctx)
						if err != nil {
							s.Fatal("Failed to restart CRAS: ", err)
						}
						if err := dlc.Install(ctx, "nc-ap-dlc", ""); err != nil {
							s.Fatal("Cannot install nc-ap-dlc: ", err)
						}
						if err := cras.SetActiveNodeByType(ctx, "INTERNAL_MIC"); err != nil {
							s.Fatal("Failed to set internal mic active: ", err)
						}
						if err := cras.SetActiveNodeByType(ctx, "INTERNAL_SPEAKER"); err != nil {
							s.Fatal("Failed to set internal speaker active: ", err)
						}
						if err := cras.SetNoiseCancellationEnabled(ctx, true); err != nil {
							s.Fatal("Failed to SetNoiseCancellationEnabled: ", err)
						}
						if err := cras.WaitUntilFeatureFlagHasValue(ctx, "CrOSLateBootAudioAPNoiseCancellation", true); err != nil {
							s.Fatal("Faild to WaitUntilFeatureFlagHasValue: ", err)
						}
					},
					extraCrasClientArgs: []string{"--effects=aec"},
				},
			},
		},
	})
}

type crasNoiseCancellationPowerParam struct {
	crasSetUp           func(ctx context.Context, s *testing.State)
	extraCrasClientArgs []string
}

// CrasNoiseCancellationPower measures the power for running noise cancellation in CRAS.
func CrasNoiseCancellationPower(ctx context.Context, s *testing.State) {
	param := s.Param().(crasNoiseCancellationPowerParam)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	param.crasSetUp(ctx, s)

	const (
		interval     = 1 * time.Second // Power metrics collect interval.
		testDuration = 5 * time.Minute
		blockSize    = 480
	)

	playbackCommand := crastestclient.PlaybackCommand(ctx, int(testDuration.Seconds()), blockSize)
	captureCommand := crastestclient.CaptureCommand(ctx, int(testDuration.Seconds()), blockSize)
	captureCommand.Args = append(captureCommand.Args, param.extraCrasClientArgs...)

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := r.Record(ctx, func(ctx context.Context) error {
		if err := playbackCommand.Start(); err != nil {
			return errors.Wrap(err, "cannot start playback command")
		}
		if err := captureCommand.Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "cannot start capture command")
		}
		if err := playbackCommand.Wait(); err != nil {
			return errors.Wrap(err, "cannot wait playback command")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to record: ", err)
	}
}
