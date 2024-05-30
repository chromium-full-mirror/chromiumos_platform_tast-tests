// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/data"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasSidetoneMultipleLoopbackLatency,
		Desc:         "Measure the sidetone latency using multiple aloop with loopback_latency",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "normanbt@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline", "informational",
			"group:cbx", "cbx_feature_enabled", "cbx_stable",
		},
		Fixture: fixture.AloopLoaded{
			Channels:    2,
			DevicePairs: 2,
		}.Instance(),
		Timeout:      3 * time.Minute,
		Data:         []string{data.TheQuickBrownFoxS16LEStereo48000Wav},
		SoftwareDeps: []string{"chrome"},
		// TODO(b/312097873): remove "brya" when b/309904720 is fixed.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("brya")),
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "no_effects",
				Val:  crasSidetoneMultipleLoopbackLatencyParams{},
			},
			{
				Name: "nc",
				Val: crasSidetoneMultipleLoopbackLatencyParams{
					noiseCancellationEnabled: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name: "nc_ast",
				Val: crasSidetoneMultipleLoopbackLatencyParams{
					noiseCancellationEnabled: true,
					styleTransferEnabled:     true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
		},
	})
}

type crasSidetoneMultipleLoopbackLatencyParams struct {
	noiseCancellationEnabled bool
	styleTransferEnabled     bool
}

// CrasSidetoneMultipleLoopbackLatency measures the sidetone latency with multiple loopback setups
// and some audio processing.
func CrasSidetoneMultipleLoopbackLatency(ctx context.Context, s *testing.State) {
	param := s.Param().(crasSidetoneMultipleLoopbackLatencyParams)
	apConfig := audio.NoiseCancellationConfig{
		NoiseCancellationEnabled: param.noiseCancellationEnabled,
		StyleTransferEnabled:     param.styleTransferEnabled,
	}
	if err := audio.WithNoiseCancellation(ctx, apConfig, s.OutDir(), s.HasError, "Loopback Capture", "Loopback Playback 1", func(ctx context.Context, cras *audio.Cras) {
		if err := cras.SetSidetoneEnabled(ctx, true); err != nil {
			s.Fatal("Failed to SetSidetoneEnabled: ", err)
		}
		defer func() {
			cras.SetSidetoneEnabled(ctx, false)
		}()

		outputPath := filepath.Join(s.OutDir(), "result.txt")
		output, err := os.Create(outputPath)
		if err != nil {
			s.Fatal("Failed to create output file: ", err)
		}
		defer output.Close()

		const (
			bufferSize     = "480"
			periodSize     = "240"
			noiseThreshold = "1000"
		)

		// The audio path is the same as CrasSidetone test.
		cmd := testexec.CommandContext(
			ctx,
			"loopback_latency",
			"-i",
			"hw:Loopback_1,1",
			"-o",
			"hw:Loopback,0",
			"-b",
			bufferSize,
			"-p",
			periodSize,
			"-n",
			noiseThreshold,
			"-f",
			s.DataPath(data.TheQuickBrownFoxS16LEStereo48000Wav),
		)
		cmd.Stdout = output
		if err := cmd.Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to run loopback_latency: ", err)
		}

		perfValues := perf.NewValues()
		defer func() {
			if err := perfValues.Save(s.OutDir()); err != nil {
				s.Error("Cannot save perf data: ", err)
			}
		}()

		result, err := audio.ParseLoopbackLatencyResult(outputPath, 1)
		if err != nil {
			s.Fatal("Failed to parse the latency result: ", err)
		}
		s.Logf("Result: %+v", result)

		audio.UpdatePerfValuesFromResult(perfValues, result, bufferSize)
	}); err != nil {
		s.Fatal("Failed to setup noise cancellation: ", err)
	}
}
