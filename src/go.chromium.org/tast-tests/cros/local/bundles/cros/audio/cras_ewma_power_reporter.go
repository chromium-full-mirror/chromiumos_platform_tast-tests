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
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasEwmaPowerReporter,
		Desc:         "Check power reporter functionality when cras is capturing audio",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "normanbt@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:release-health",
			"release-health_audio",
		},
		VariantCategory: `{"name": "Audio_Board"}`,
		HardwareDeps:    hwdep.D(hwdep.SkipOnModel("betty")),
		Fixture: fixture.AloopLoaded{
			Channels: 2,
			Parent:   fixture.UIStopped{}.Instance(),
		}.Instance(),
		Timeout: 3 * time.Minute,
	})
}

// CrasEwmaPowerReporter checks power reporter functionality when cras is capturing audio
func CrasEwmaPowerReporter(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}
	if err := audio.SelectIODevices(ctx, cras, nodematch.Type("ALSA_LOOPBACK"), nodematch.Type("ALSA_LOOPBACK")); err != nil {
		s.Fatal("Cannot select IO devices: ", err)
	}

	if err := cras.SetEwmaPowerReportEnabled(ctx, true); err != nil {
		s.Fatal("Failed to SetEwmaPowerReportEnabled: ", err)
	}
	defer cras.SetEwmaPowerReportEnabled(ctx, false)

	signalWatcher, err := dbusutil.NewSignalWatcherForSystemBus(ctx, dbusutil.MatchSpec{
		Type:      "signal",
		Interface: "org.chromium.cras.Control",
		Member:    "EwmaPowerReported",
	})
	if err != nil {
		s.Fatal("Cannot set up a D-Bus signal watcher for EwmaPowerReported")
	}
	defer signalWatcher.Close(cleanupCtx)

	var powers []float64
	go func() {
		for sig := range signalWatcher.Signals {
			power, ok := sig.Body[0].(float64)
			if !ok {
				s.Fatal("Detected power is not float, got: ", sig)
			}
			powers = append(powers, power)
		}
	}()

	const (
		wavDuration            = 10 * time.Second
		startingLatencyAllowed = 1 * time.Second
	)

	playbackWavPath := filepath.Join(s.OutDir(), "sine.wav")
	playbackWavData := audio.TestRawData{
		Path:          playbackWavPath,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Frequencies:   []int{440, 440},
		Volume:        0.8,
		Duration:      int(wavDuration.Seconds()),
	}
	if err := audio.GenerateTestWavData(ctx, playbackWavData); err != nil {
		s.Fatal("Failed to generate sine wav file: ", err)
	}

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

	captureWavPath := filepath.Join(s.OutDir(), "capture.wav")
	captureWavData := audio.TestRawData{
		Path:          captureWavPath,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Duration:      int(wavDuration.Seconds()),
	}
	if err := audio.CaptureWavFromDefault(playbackCaptureCtx, captureWavData); err != nil {
		s.Fatal("Cannot run capture: ", err)
	}
	s.Log("Capture complete")

	var captureWav wav.File
	if captureWav, err = wav.ReadPCMFile(ctx, captureWavPath); err != nil {
		s.Fatalf("Cannot read %s, err: %v", captureWavPath, err)
	}
	audioData := captureWav.GetBodyAsInt16()
	for ch := 0; ch < 2; ch++ {
		if err := audio.CheckFrequency(ctx, audioData[ch], 48000 /*sampleRate*/, 440 /*expectedFreq*/, 10 /*freqTolerance*/, 10 /*incorrectLimit*/, startingLatencyAllowed); err != nil {
			s.Error("CheckFrequency err: ", err)
		}
	}

	s.Log("Waiting for playback to complete")
	<-playbackDone

	detected := false
	for _, power := range powers {
		if power > 0 {
			detected = true
			break
		}
	}

	if !detected {
		s.Fatal("Ewma power not detected. Powers: ", powers)
	}
}
