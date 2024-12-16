// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastests"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasPlaybackPower,
		Desc:         "Collect power metrics of audio playback with various configs in CRAS",
		BugComponent: "b:776546",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "johnylin@google.com"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		Attr:         []string{
			"group:crosbolt",
			"crosbolt_perbuild",
			"group:release-health",
			"release-health_audio",
		},
		Timeout:      25*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{
			{
				Name:    "baseline",
				Fixture: "powerAshPlatformAudio",
				Val: crasPlaybackPowerParam{
					crasSetUp: func(ctx context.Context, s *testing.State) {
						cras, err := audio.RestartCras(ctx)
						if err != nil {
							s.Fatal("Failed to restart CRAS: ", err)
						}

						node, err := selectInternalSpeakerInMaxVolume(ctx, cras)
						if err != nil {
							s.Fatal("Failed to select internal speaker: ", err)
						}

						supported, err := cras.GetDSPOffloadSupported(ctx, *node)
						if err != nil {
							s.Log("WARN: failed to get offload support flag: ", err)
						} else {
							s.Log("DSP offload support flag: ", supported)
						}
					},
				},
			},
		},
	})
}

type crasPlaybackPowerParam struct {
	crasSetUp func(ctx context.Context, s *testing.State)
}

func selectInternalSpeakerInMaxVolume(ctx context.Context, cras *audio.Cras) (*audio.CrasNode, error) {
	const expectedNodeType = "INTERNAL_SPEAKER"

	if err := cras.SetActiveNodeByType(ctx, expectedNodeType); err != nil {
		return nil, errors.Wrap(err, "failed to set internal speaker active")
	}

	node, err := cras.SelectedOutputNode(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get active node")
	}
	if node.Type != expectedNodeType {
		return nil, errors.New("the active node is not internal speaker")
	}

	if err := cras.SetOutputNodeVolume(ctx, *node, 100); err != nil {
		return nil, errors.Wrap(err, "failed to set volume")
	}
	return node, nil
}

// CrasPlaybackPower measures the power for audio playback in CRAS.
func CrasPlaybackPower(ctx context.Context, s *testing.State) {
	param := s.Param().(crasPlaybackPowerParam)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	param.crasSetUp(ctx, s)

	const (
		interval     = 5 * time.Second // Power metrics collect interval.
		testDuration = 20 * time.Minute
		bufferSize   = 480
	)

	playbackCommand := crastests.PlaybackCommand(ctx, int(testDuration.Seconds()), bufferSize)

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := r.Record(ctx, func(ctx context.Context) error {
		if err := playbackCommand.Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "cannot run playback command")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to record: ", err)
	}
}
