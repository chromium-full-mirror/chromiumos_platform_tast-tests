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
			"group:cbx", "cbx_feature_enabled", "cbx_stable",
		},
		Fixture:      fixture.AloopLoaded{Channels: 2}.Instance(),
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		// TODO(b/312097873): remove "brya" when b/309904720 is fixed.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("brya")),
		LacrosStatus: testing.LacrosVariantUnneeded,
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
					noiseCancellationEnabled: true,
					styleTransferEnabled:     true,
					captureRate:              48000,
					expectedRMS:              0.0075,
					expectedRMSTolerance:     0.0075,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "aec_nc_ast_44100hz",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					styleTransferEnabled:     true,
					captureRate:              44100,
					expectedRMS:              0.0075,
					expectedRMSTolerance:     0.0075,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc_ast",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					styleTransferEnabled:     true,
					captureRate:              48000,
					expectedRMS:              0.01,
					expectedRMSTolerance:     0.005,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc_ast_44100hz",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					styleTransferEnabled:     true,
					captureRate:              44100,
					expectedRMS:              0.01,
					expectedRMSTolerance:     0.005,
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
		NoiseCancellationEnabled: param.noiseCancellationEnabled,
		StyleTransferEnabled:     param.styleTransferEnabled,
		ChromeOpts:               param.extraChromeOpts,
	}

	if err := audio.WithNoiseCancellation(ctx, apConfig, s.OutDir(), s.HasError, "Loopback Playback", "Loopback Capture", func(ctx context.Context, _ *audio.Cras) {
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
	}); err != nil {
		s.Fatal("Failed to setup noise cancellation: ", err)
	}
}
