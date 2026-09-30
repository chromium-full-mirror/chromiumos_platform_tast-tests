// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast-tests/cros/local/audio/wav"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/data"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasSidetonePureLatency,
		Desc:         "Measures the pure latency of the sidetone",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "normanbt@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"informational",
		},
		VariantCategory: `{"name": "Audio_Board"}`,
		Data:            []string{data.AudioShortSine440Wav},
		HardwareDeps:    hwdep.D(hwdep.SkipOnModel("betty")),
		Fixture: fixture.AloopLoaded{
			Channels: 2,
			Parent:   fixture.UIStopped{}.Instance(),
		}.Instance(),
		Timeout: 3 * time.Minute,
	})
}

// CrasSidetonePureLatency measures the pure latency of the sidetone.
// It plays a short sine wave to the loopback device. And because of the sidetone,
// the sinewave will be repeated over and over again. And the latency will be the distance
// between the first captured sine wave and the second captured sinve wave.
// The path of the audio is as follows:
//
//	                      ->  aloop capture       (cras) ->
//	                    /                      \            \
//	sine.wav \      (snd-aloop)        (cras sidetone)       \  capture.wav
//	          \         \                      /
//	           ->(cras)    aloop playback   <-
func CrasSidetonePureLatency(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}

	if err := audio.SelectIODevices(ctx, cras, nodematch.Type("ALSA_LOOPBACK"), nodematch.Type("ALSA_LOOPBACK")); err != nil {
		s.Fatal("Cannot select IO devices: ", err)
	}

	if err := cras.SetSidetoneEnabled(ctx, true); err != nil {
		s.Fatal("Failed to SetSidetoneEnabled: ", err)
	}
	defer cras.SetSidetoneEnabled(ctx, false)

	const (
		wavDuration     = 2 * time.Second
		sineWaveLength  = 5 * time.Millisecond
		frameRate       = 48000
		expectedLatency = 20 * time.Millisecond // It should be constant between tests, unless there is a CRAS change
		tolerance       = 3 * time.Millisecond
	)

	playbackWavPath := s.DataPath(data.AudioShortSine440Wav)
	playbackCaptureCtx, cancel := context.WithTimeout(ctx, 2*wavDuration)
	defer cancel()

	playbackDone := make(chan struct{})
	go func() {
		defer close(playbackDone)
		if err := audio.PlayWavToDefault(playbackCaptureCtx, playbackWavPath); err != nil {
			s.Error("Cannot run playback: ", err)
		}
		s.Log("Playback complete")
	}()

	captureWavData := audio.TestRawData{
		Path:          filepath.Join(s.OutDir(), "capture.wav"),
		BitsPerSample: 16,
		Channels:      2,
		Rate:          frameRate,
		Duration:      int(wavDuration.Seconds()),
	}

	if err := audio.CaptureWavFromDefault(ctx, captureWavData); err != nil {
		s.Fatal("Cannot run capture: ", err)
	}
	s.Log("Capture complete")

	var captureWav wav.File
	if captureWav, err = wav.ReadPCMFile(ctx, captureWavData.Path); err != nil {
		s.Fatalf("Cannot read %s, err: %v", captureWavData.Path, err)
	}

	firstNonZero := getFirstNonZeroFrame(captureWav.Body[0])
	offset := firstNonZero + int(sineWaveLength.Milliseconds())*(frameRate/1000)
	secondNonZero := getFirstNonZeroFrame(captureWav.Body[0][offset:]) + offset

	latency := time.Duration(secondNonZero-firstNonZero) * time.Millisecond / time.Duration(frameRate/1000)

	s.Logf("First: %d, Second: %d", firstNonZero, secondNonZero)
	s.Logf("Latency: %d ms", latency.Milliseconds())

	if latency > expectedLatency+tolerance {
		s.Errorf("Latency (%d) is greater than expected (%d)", latency.Milliseconds(), expectedLatency.Milliseconds())
	}

	s.Log("Waiting for playback to complete")
	<-playbackDone
}

func getFirstNonZeroFrame(pcmData []int32) int {
	for i, data := range pcmData {
		if data != 0 {
			return i
		}
	}
	return -1
}
