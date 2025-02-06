// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio/crastests"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var hwdepDSPModels = hwdep.Model("redrix", "gimble", "anahera")

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasNoiseCancellationPower,
		Desc:         "Collect power metrics of using noise cancellation in CRAS",
		BugComponent: "b:776546",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Speaker()),
		Timeout:      10*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{
			{
				Name: "no_effects",
				Val: crasNoiseCancellationPowerParam{
					extraCrasClientArgs: nil,
				},
				Fixture: fixture.CrasSetUp{
					ChromeFixture: setup.PowerAshPlatformAudioNoiseCancellation,
					InputDevice:   nodematch.Type("INTERNAL_MIC"),
					OutputDevice:  nodematch.Type("INTERNAL_SPEAKER"),
				}.Instance(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "aec",
				Val: crasNoiseCancellationPowerParam{
					extraCrasClientArgs: []string{"--effects=0x1"},
				},
				Fixture: fixture.CrasSetUp{
					ChromeFixture:           setup.PowerAshPlatformAudioNoiseCancellation,
					InputDevice:             nodematch.Type("INTERNAL_MIC"),
					OutputDevice:            nodematch.Type("INTERNAL_SPEAKER"),
					VoiceIsolationUIEnabled: false,
				}.Instance(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "aec_nc",
				Val: crasNoiseCancellationPowerParam{
					extraCrasClientArgs: []string{"--effects=0x1"},
				},
				Fixture: fixture.CrasSetUp{
					ChromeFixture:           setup.PowerAshPlatformAudioNoiseCancellation,
					InputDevice:             nodematch.Type("INTERNAL_MIC"),
					OutputDevice:            nodematch.Type("INTERNAL_SPEAKER"),
					VoiceIsolationUIEnabled: true,
					CrasFeatures: fixture.CrasFeatureOverrides{
						fixture.APNoiseCancellation: true,
						fixture.StyleTransfer:       false,
					},
				}.Instance(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "aec_nc_ast",
				Val: crasNoiseCancellationPowerParam{
					extraCrasClientArgs: []string{"--effects=0x1"},
				},
				Fixture: fixture.CrasSetUp{
					ChromeFixture:           setup.PowerAshPlatformAudioStyleTransfer,
					InputDevice:             nodematch.Type("INTERNAL_MIC"),
					OutputDevice:            nodematch.Type("INTERNAL_SPEAKER"),
					VoiceIsolationUIEnabled: true,
					CrasFeatures: fixture.CrasFeatureOverrides{
						fixture.APNoiseCancellation: true,
						fixture.StyleTransfer:       true,
					},
				}.Instance(),
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "dsp_aec",
				Val: crasNoiseCancellationPowerParam{
					extraCrasClientArgs: []string{"--effects=0x11"},
				},
				Fixture: fixture.CrasSetUp{
					ChromeFixture: setup.PowerAshPlatformAudioNoiseCancellation,
					InputDevice:   nodematch.Type("INTERNAL_MIC"),
					OutputDevice:  nodematch.Type("INTERNAL_SPEAKER"),
					CrasFeatures:  fixture.CrasFeatureOverrides{},
				}.Instance(),
				ExtraHardwareDeps: hwdep.D(hwdepDSPModels),
			},
			{
				Name: "dsp_aec_nc",
				Val: crasNoiseCancellationPowerParam{
					extraCrasClientArgs: []string{"--effects=0x11"},
				},
				Fixture: fixture.CrasSetUp{
					ChromeFixture:           setup.PowerAshPlatformAudioNoiseCancellation,
					InputDevice:             nodematch.Type("INTERNAL_MIC"),
					OutputDevice:            nodematch.Type("INTERNAL_SPEAKER"),
					VoiceIsolationUIEnabled: true,
					CrasFeatures:            fixture.CrasFeatureOverrides{},
				}.Instance(),
				ExtraHardwareDeps: hwdep.D(hwdepDSPModels),
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
		},
	})
}

type crasNoiseCancellationPowerParam struct {
	extraCrasClientArgs []string
}

// CrasNoiseCancellationPower measures the power for running noise cancellation in CRAS.
func CrasNoiseCancellationPower(ctx context.Context, s *testing.State) {
	param := s.Param().(crasNoiseCancellationPowerParam)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	const (
		interval     = 5 * time.Second // Power metrics collect interval.
		testDuration = 5 * time.Minute
		bufferSize   = 480
	)

	playbackCommand := crastests.PlaybackCommand(ctx, int(testDuration.Seconds()), bufferSize)
	captureCommand := crastests.CaptureCommand(ctx, int(testDuration.Seconds()), bufferSize)
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
