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
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
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
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              48000,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc_44100hz",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              44100,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "aec_nc_ast",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              48000,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
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
					captureRate:              44100,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
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
					captureRate:              48000,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc_ast_44100hz",
				Val: crasNoiseCancellationParams{
					noiseCancellationEnabled: true,
					captureRate:              44100,
					expectedRMS:              0.03,
					expectedRMSTolerance:     0.01,
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

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	// Start chrome.
	chromeOpts := param.extraChromeOpts
	if param.styleTransferEnabled {
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("CrOSLateBootAudioStyleTransfer"))
	}
	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Install DLC.
	if param.noiseCancellationEnabled {
		if err := dlc.Install(ctx, "nc-ap-dlc", ""); err != nil {
			s.Fatal("Cannot install nc-ap-dlc: ", err)
		}
	}
	if param.styleTransferEnabled {
		if err := dlc.Install(ctx, "nuance-dlc", ""); err != nil {
			s.Fatal("Cannot install nuance-dlc: ", err)
		}
	}

	// Start Cras.
	if err := audio.SetupLoopback(ctx, cr); err != nil {
		s.Fatal("Failed to SetupLoopback: ", err)
	}

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}
	if param.styleTransferEnabled {
		if err := cras.WaitUntilFeatureFlagHasValue(ctx, "CrOSLateBootAudioStyleTransfer", true); err != nil {
			s.Fatal("Feature flag not propagated to CRAS: ", err)
		}
	}
	if err := cras.SetNoiseCancellationEnabled(ctx, param.noiseCancellationEnabled); err != nil {
		s.Fatal("Failed to SetNoiseCancellationEnabled: ", err)
	}
	if err := cras.SetStyleTransferEnabled(ctx, param.styleTransferEnabled); err != nil {
		s.Fatal("Failed to SetStyleTransferEnabled: ", err)
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
}
