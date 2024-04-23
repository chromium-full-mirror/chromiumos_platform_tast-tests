// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/audio/wav"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/data"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasAEC,
		Desc:         "Check AEC in CRAS using aloop",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"informational", "group:criticalstaging",
		},
		Data:         []string{data.AudioLong16Wav},
		Timeout:      3 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "48000_mono",
				Val: crasAECParams{
					rate:     48000,
					channels: 1,
				},
				Fixture: crasAECFixture(1),
			},
			{
				Name: "44100_mono",
				Val: crasAECParams{
					rate:     44100,
					channels: 1,
				},
				Fixture: crasAECFixture(1),
			},
			{
				Name: "16000_mono",
				Val: crasAECParams{
					rate:     16000,
					channels: 1,
				},
				Fixture: crasAECFixture(1),
			},
			{
				Name: "48000_stereo",
				Val: crasAECParams{
					rate:     48000,
					channels: 2,
				},
				Fixture: crasAECFixture(2),
			},
			{
				Name: "44100_stereo",
				Val: crasAECParams{
					rate:     44100,
					channels: 2,
				},
				Fixture: crasAECFixture(2),
			},
			{
				Name: "16000_stereo",
				Val: crasAECParams{
					rate:     16000,
					channels: 2,
				},
				Fixture: crasAECFixture(2),
			},
		},
	})
}

type crasAECParams struct {
	rate     int
	channels int
}

var crasAECChromeFixture = fixture.Chrome(chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"))

func crasAECFixture(channels int) string {
	return fixture.AloopLoaded{
		Channels: channels,
		Parent:   crasAECChromeFixture,
	}.Instance()
}

func CrasAEC(ctx context.Context, s *testing.State) {
	param := s.Param().(crasAECParams)

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}

	if err := audio.SelectIODevices(ctx, cras, "ALSA_LOOPBACK", "ALSA_LOOPBACK"); err != nil {
		s.Fatal("Cannot select ALSA loopback: ", err)
	}

	const captureDuration = 10 * time.Second
	const playbackDuration = 2*time.Second + captureDuration

	playbackWav := filepath.Join(s.OutDir(), "playback.wav")
	if err := wav.RepeatForDuration(
		ctx,
		s.DataPath(data.AudioLong16Wav), playbackWav, playbackDuration,
		wav.Channels(param.channels), wav.Rate(param.rate),
	); err != nil {
		s.Fatalf("Cannot prepare %s: %v", playbackWav, err)
	}

	playbackCaptureCtx, cancel := context.WithTimeout(ctx, 2*playbackDuration)
	defer cancel()

	playbackDone := make(chan struct{})
	go func() {
		defer close(playbackDone)
		// Run playback.
		s.Log("Playing ", playbackWav)
		if err := testexec.CommandContext(playbackCaptureCtx,
			"cras_tests",
			"playback",
			"--buffer_size=480",
			fmt.Sprintf("--rate=%d", param.rate),
			fmt.Sprintf("--channels=%d", param.channels),
			playbackWav,
		).Run(testexec.DumpLogOnError); err != nil {
			s.Error("Cannot run playback: ", err)
		}
	}()

	// Run capture.
	captureWav := filepath.Join(s.OutDir(), "capture.wav")
	s.Log("Capturing ", captureWav)
	if err := testexec.CommandContext(
		playbackCaptureCtx,
		"cras_tests",
		"capture",
		"--effects=0x1",
		"--buffer_size=480",
		fmt.Sprintf("--rate=%d", param.rate),
		fmt.Sprintf("--channels=%d", param.channels),
		fmt.Sprintf("--duration=%g", captureDuration.Seconds()),
		captureWav,
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Cannot run capture: ", err)
	}

	rms, err := audio.GetRmsAmplitudeFromWav(ctx, captureWav)
	if err != nil {
		s.Fatal("Failed GetRmsAmplitudeFromWav: ", err)
	}

	s.Log("RMS: ", rms)
	if rms < 1e-6 {
		s.Fatal("RMS too small: ", rms)
	}
	const wantRMSLess = 0.005
	if rms >= wantRMSLess {
		s.Fatalf("RMS too big: got %v, want < %v", rms, wantRMSLess)
	}

	s.Log("Waiting for playback to complete")
	<-playbackDone
}
