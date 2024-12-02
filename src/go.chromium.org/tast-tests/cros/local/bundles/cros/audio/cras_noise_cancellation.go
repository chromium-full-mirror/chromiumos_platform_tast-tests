// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/device"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasNoiseCancellation,
		Desc:         "Check noise cancellation in CRAS using aloop",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_stable",
			"group:release-health",
			"release-health_audio",
		},
		Fixture:      fixture.AloopLoaded{Channels: 2}.Instance(),
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name: "no_effects",
				Val: crasNoiseCancellationParams{
					captureRate:          48000,
					expectedRMS:          0.35,
					expectedRMSTolerance: 0.15,
				},
			},
			{
				Name: "aec",
				Val: crasNoiseCancellationParams{
					captureRate:          48000,
					expectedRMS:          0.3,
					expectedRMSTolerance: 0.2,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
			},
			{
				Name: "aec_nc",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              48000,
					expectedRMS:              0.01,
					expectedRMSTolerance:     0.005,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "aec_nc_44100hz",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              44100,
					expectedRMS:              0.01,
					expectedRMSTolerance:     0.005,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              48000,
					expectedRMS:              0.01,
					expectedRMSTolerance:     0.005,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc_44100hz",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              44100,
					expectedRMS:              0.01,
					expectedRMSTolerance:     0.005,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "aec_nc_ast",
				Val: crasNoiseCancellationParams{
					styleTransferEnabled: true,
					captureRate:          48000,
					expectedRMS:          0.0075,
					expectedRMSTolerance: 0.0075,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "aec_nc_ast_44100hz",
				Val: crasNoiseCancellationParams{
					styleTransferEnabled: true,
					captureRate:          44100,
					expectedRMS:          0.0075,
					expectedRMSTolerance: 0.0075,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc_ast",
				Val: crasNoiseCancellationParams{
					styleTransferEnabled: true,
					captureRate:          48000,
					expectedRMS:          0.01,
					expectedRMSTolerance: 0.005,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc_ast_44100hz",
				Val: crasNoiseCancellationParams{
					styleTransferEnabled: true,
					captureRate:          44100,
					expectedRMS:          0.01,
					expectedRMSTolerance: 0.005,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
		},
	})
}

type crasNoiseCancellationParams struct {
	noiseCancellationEnabled bool
	styleTransferEnabled     bool
	captureRate              int
	expectedRMS              float64
	expectedRMSTolerance     float64
	extraCaptureFlags        []string
	extraChromeOpts          []chrome.Option
}

// CrasNoiseCancellation checks noise cancellation in CRAS using aloop.
func CrasNoiseCancellation(ctx context.Context, s *testing.State) {
	param := s.Param().(crasNoiseCancellationParams)
	apConfig := audio.NoiseCancellationConfig{
		StyleTransferAllowed: param.styleTransferEnabled,
		VoiceIsolation:       param.noiseCancellationEnabled || param.styleTransferEnabled,
		ChromeOpts: append(
			param.extraChromeOpts,
			// Workaround for b/377736374.
			// Here we make mock Chrome's cras client so the test has full control via D-Bus.
			chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
		),
	}

	if err := audio.WithNoiseCancellation(ctx, apConfig, s.OutDir(), s.HasError,
		// Workaround for b/377736374.
		// Fake devices to select.
		"Microphone (internal)", "Speaker (internal)",
		func(ctx context.Context, cras *audio.Cras) {
			// Workaround for b/377736374.
			// Select the audio device via D-Bus.
			if err := audio.SelectIODevices(ctx, cras, nodematch.Type("ALSA_LOOPBACK"), nodematch.Type("ALSA_LOOPBACK")); err != nil {
				s.Fatal("audio.SelectIODevices: ", err)
			}

			// Generate test file.
			const noiseDuration = 10 * time.Second

			noiseWave := filepath.Join(s.OutDir(), "noise.wav")
			if err := testexec.CommandContext(
				ctx,
				"sox",
				"-n", "-L",
				"-e", "signed-integer",
				"-b", "16",
				"-r", strconv.Itoa(param.captureRate),
				"-c", "2",
				noiseWave,
				"synth", strconv.FormatFloat(noiseDuration.Seconds(), 'f', -1, 64),
				"whitenoise",
				"gain", "-10",
			).Run(testexec.DumpLogOnError); err != nil {
				s.Fatal("Cannot generate noise.wav: ", err)
			}

			playbackCaptureCtx, cancel := context.WithTimeout(ctx, 2*noiseDuration)
			defer cancel()

			playbackDone := make(chan struct{})
			go func() {
				defer close(playbackDone)
				// Run playback.
				if err := audio.PlayWavToPCM(playbackCaptureCtx, noiseWave, device.AloopPlaybackPCM); err != nil {
					s.Error("Cannot run playback: ", err)
				}
			}()

			// Run capture.
			captureRaw := filepath.Join(s.OutDir(), "capture.raw")
			if err := testexec.CommandContext(
				playbackCaptureCtx,
				"cras_test_client",
				append(
					[]string{
						"-C", captureRaw,
						"--block_size=480",
						fmt.Sprintf("--rate=%d", param.captureRate),
						"--num_channels=1",
						fmt.Sprintf("--duration=%.0f", noiseDuration.Seconds()),
					},
					param.extraCaptureFlags...,
				)...,
			).Run(testexec.DumpLogOnError); err != nil {
				s.Error("Cannot run capture: ", err)
			}
			rawData := audio.TestRawData{
				Path:          captureRaw,
				BitsPerSample: 16,
				Channels:      1,
				Rate:          param.captureRate,
			}
			captureWav := filepath.Join(s.OutDir(), "capture.wav")
			if err := audio.ConvertRawToWav(ctx, rawData, captureWav); err != nil {
				s.Errorf("Cannot convert %s to %s: %v", captureRaw, captureWav, err)
			}

			// Verify: RMS.
			rms, err := audio.GetRmsAmplitude(ctx, audio.TestRawData{
				Path:          captureRaw,
				BitsPerSample: 16,
				Channels:      1,
				Rate:          param.captureRate,
			})
			if err != nil {
				s.Fatal("Cannot get RMS from capture.raw")
			}
			s.Log("Capture RMS: ", rms)
			if diff := rms - param.expectedRMS; math.Abs(diff) > param.expectedRMSTolerance {
				s.Fatalf("RMS %g is not within %g±%g (diff: %+g)",
					rms,
					param.expectedRMS,
					param.expectedRMSTolerance,
					diff,
				)
			}

			s.Log("Waiting for playback to complete")
			<-playbackDone
		},
	); err != nil {
		s.Fatal("Failed to setup noise cancellation: ", err)
	}
}
