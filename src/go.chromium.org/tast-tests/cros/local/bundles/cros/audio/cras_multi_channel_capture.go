// Copyright 2024 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/device"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasMultiChannelCapture,
		Desc:         "Verifies recorded samples from CRAS are correct",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      3 * time.Minute,
		Data:         []string{data.AudioLong16Wav},
		Params: []testing.Param{
			{
				Name: "1ch",
				Val: crasMultiChannelCaptureParam{
					channels: 1,
				},
				Fixture: fixture.AloopLoaded{
					Channels: 1,
					Parent:   fixture.UIStopped{}.Instance(),
				}.Instance(),
			},
			{
				Name: "2ch",
				Val: crasMultiChannelCaptureParam{
					channels: 2,
				},
				Fixture: fixture.AloopLoaded{
					Channels: 2,
					Parent:   fixture.UIStopped{}.Instance(),
				}.Instance(),
			},
			{
				Name: "3ch",
				Val: crasMultiChannelCaptureParam{
					channels: 3,
				},
				Fixture: fixture.AloopLoaded{
					Channels: 3,
					Parent:   fixture.UIStopped{}.Instance(),
				}.Instance(),
			},
			{
				Name: "1ch_aec",
				Val: crasMultiChannelCaptureParam{
					channels: 1,
					effects:  0x1,
				},
				Fixture: fixture.AloopLoaded{
					Channels: 1,
					Parent:   fixture.UIStopped{}.Instance(),
				}.Instance(),
			},
			{
				Name: "2ch_aec",
				Val: crasMultiChannelCaptureParam{
					channels: 2,
					effects:  0x1,
				},
				Fixture: fixture.AloopLoaded{
					Channels: 2,
					Parent:   fixture.UIStopped{}.Instance(),
				}.Instance(),
			},
			{
				Name: "3ch_aec",
				Val: crasMultiChannelCaptureParam{
					channels: 3,
					effects:  0x1,
				},
				Fixture: fixture.AloopLoaded{
					Channels: 3,
					Parent:   fixture.UIStopped{}.Instance(),
				}.Instance(),
			},
		},
	})
}

type crasMultiChannelCaptureParam struct {
	channels int
	effects  int
}

func CrasMultiChannelCapture(ctx context.Context, s *testing.State) {
	param := s.Param().(crasMultiChannelCaptureParam)

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}

	if err := audio.SelectIODevices(ctx, cras, "ALSA_LOOPBACK", "ALSA_LOOPBACK"); err != nil {
		s.Fatal("Cannot select IO devices: ", err)
	}

	const testDuration = 10 * time.Second

	var clips []string

	for ch := 0; ch < param.channels; ch++ {
		clip := filepath.Join(s.OutDir(), fmt.Sprintf("clip%d.wav", ch))
		startTime := testDuration * time.Duration(ch)
		if err := testexec.CommandContext(
			ctx,
			"sox",
			s.DataPath(data.AudioLong16Wav),
			"--rate=48000",
			clip,
			"trim",
			strconv.FormatFloat(startTime.Seconds(), 'f', 0, 64),
			strconv.FormatFloat(testDuration.Seconds(), 'f', 0, 64),
		).Run(testexec.DumpLogOnError); err != nil {
			s.Fatalf("Cannot create channel %d clip: %v", ch, err)
		}

		clips = append(clips, clip)
	}

	mergedWav := filepath.Join(s.OutDir(), "merged.wav")
	cmd := testexec.CommandContext(ctx, "sox")
	if param.channels > 1 {
		cmd.Args = append(cmd.Args, "-M")
	}
	cmd.Args = append(cmd.Args, clips...)
	cmd.Args = append(cmd.Args, mergedWav)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Cannot create merged wav: ", err)
	}

	playbackCaptureCtx, cancel := context.WithTimeout(ctx, testDuration*2)
	defer cancel()

	playbackDone := make(chan struct{})
	go func() {
		defer close(playbackDone)
		if err := audio.PlayWavToPCM(playbackCaptureCtx, mergedWav, device.AloopPlaybackPCM); err != nil {
			s.Fatal("PlayWavToPCM failed: ", err)
		}
	}()

	captureWav := filepath.Join(s.OutDir(), "capture.wav")
	if err := testexec.CommandContext(
		playbackCaptureCtx,
		"cras_tests",
		"capture",
		captureWav,
		fmt.Sprintf("--effects=0x%x", param.effects),
		fmt.Sprintf("--channels=%d", param.channels),
		fmt.Sprintf("--duration=%v", testDuration.Seconds()),
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("cras_tests capture failed: ", err)
	}

	<-playbackDone
	// TODO(b/261951580): Actually verify the capture content.
}
