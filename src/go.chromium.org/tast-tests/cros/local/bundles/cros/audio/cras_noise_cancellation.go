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
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasNoiseCancellation,
		Desc:         "Check noise cancellation in CRAS using aloop",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"group:video_conference", "video_conference_per_build",
		},
		Fixture:      fixture.AloopLoaded{Channels: 2}.Instance(),
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"chrome"},
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
					expectedRMS:          0.35,
					expectedRMSTolerance: 0.15,
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
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
				ExtraSoftwareDeps: []string{"ap_noise_cancellation"},
			},
			{
				Name: "aec_nc_44100hz",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              44100,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
					extraCaptureFlags: []string{
						"--effects=aec",
					},
				},
				ExtraSoftwareDeps: []string{"ap_noise_cancellation"},
			},
			{
				Name: "nc",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              48000,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
				},
				ExtraSoftwareDeps: []string{"ap_noise_cancellation"},
			},
			{
				Name: "nc_44100hz",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              44100,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
				},
				ExtraSoftwareDeps: []string{"ap_noise_cancellation"},
			},
		},
	})
}

type crasNoiseCancellationParams struct {
	noiseCancellationEnabled bool
	captureRate              int
	expectedRMS              float64
	expectedRMSTolerance     float64
	extraCaptureFlags        []string
	extraChromeOpts          []chrome.Option
}

// CrasNoiseCancellation checks noise cancellation in CRAS using aloop.
func CrasNoiseCancellation(ctx context.Context, s *testing.State) {
	param := s.Param().(crasNoiseCancellationParams)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	chromeOpts := append(param.extraChromeOpts, chrome.EnableFeatures("QsRevamp"))
	if param.noiseCancellationEnabled {
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("CrOSLateBootAudioAPNoiseCancellation"))
	}
	cr, err := chrome.New(ctx, chromeOpts...)
	defer cr.Close(cleanupCtx)

	if param.noiseCancellationEnabled {
		if err := dlc.Install(ctx, "nc-ap-dlc", ""); err != nil {
			s.Fatal("Cannot install nc-ap-dlc: ", err)
		}
	}

	if err := audio.SetupLoopback(ctx, cr); err != nil {
		s.Fatal("Failed to SetupLoopback: ", err)
	}

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}
	if param.noiseCancellationEnabled {
		if err := cras.WaitUntilFeatureFlagHasValue(ctx, "CrOSLateBootAudioAPNoiseCancellation", true); err != nil {
			s.Fatal("Feature flag not propagated to CRAS: ", err)
		}
	}
	if err := cras.SetNoiseCancellationEnabled(ctx, param.noiseCancellationEnabled); err != nil {
		s.Fatal("Failed to SetNoiseCancellationEnabled: ", err)
	}

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
		"sine", "300",
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
	captureWav := filepath.Join(s.OutDir(), "capture.wav")
	if err := audio.ConvertRawToWav(ctx, captureRaw, captureWav, param.captureRate, 1); err != nil {
		s.Errorf("Cannot convert %s to %s: %v", captureRaw, captureWav, err)
	}

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
}
