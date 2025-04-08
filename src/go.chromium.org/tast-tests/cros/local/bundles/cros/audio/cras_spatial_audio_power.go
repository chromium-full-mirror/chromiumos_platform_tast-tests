// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

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

var hwdepSpatialAudioModels = hwdep.Model("rauru", "navi")

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasSpatialAudioPower,
		Desc:         "Collect power metrics of using spatial audio in CRAS",
		BugComponent: "b:776546",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "eddyhsu@google.com"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Speaker()),
		Timeout:      10*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{
			{
				Name: "no_spatial_audio",
				Fixture: fixture.CrasSetUp{
					ChromeFixture: setup.PowerAshPlatformAudio,
					InputDevice:   nodematch.Type("INTERNAL_MIC"),
					OutputDevice:  nodematch.Type("INTERNAL_SPEAKER"),
					SpatialAudio:  false,
				}.Instance(),
				ExtraHardwareDeps: hwdep.D(hwdepSpatialAudioModels),
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "spatial_audio",
				Fixture: fixture.CrasSetUp{
					ChromeFixture: setup.PowerAshPlatformAudio,
					InputDevice:   nodematch.Type("INTERNAL_MIC"),
					OutputDevice:  nodematch.Type("INTERNAL_SPEAKER"),
					SpatialAudio:  true,
				}.Instance(),
				ExtraHardwareDeps: hwdep.D(hwdepSpatialAudioModels),
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
		},
	})
}

// CrasSpatialAudioPower measures the power for running spatial audio in CRAS.
func CrasSpatialAudioPower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	const (
		interval     = 5 * time.Second // Power metrics collect interval.
		testDuration = 5 * time.Minute
		bufferSize   = 4096
	)

	playbackCommand := crastests.PlaybackCommand(ctx, int(testDuration.Seconds()), bufferSize)

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := r.Record(ctx, func(ctx context.Context) error {
		if err := playbackCommand.Start(); err != nil {
			return errors.Wrap(err, "cannot start playback command")
		}
		if err := playbackCommand.Wait(); err != nil {
			return errors.Wrap(err, "cannot wait playback command")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to record: ", err)
	}
}
