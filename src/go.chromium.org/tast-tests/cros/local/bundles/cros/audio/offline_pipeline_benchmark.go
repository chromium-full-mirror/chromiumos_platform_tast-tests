// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OfflinePipelineBenchmark,
		Desc:         "Benchmark audio_processor modules using offline-pipeline",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "paulhsia@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:776546",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Timeout:      8 * time.Minute,
		SoftwareDeps: []string{
			"chrome", // For DLC.
		},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "nc",
				Val: offlinePipelineBenchmarkParam{
					dlcID:             "nc-ap-dlc",
					dlcSharedObject:   "libdenoiser.so",
					pluginName:        "plugin_processor_create",
					blockSizeFrames:   480,
					inputWavFrameRate: 48000,
				},
				ExtraSoftwareDeps: []string{
					"amd64", // libdenoiser.so is amd64 only.
				},
			},
			{
				Name: "ast",
				Val: offlinePipelineBenchmarkParam{
					dlcID:             "nuance-dlc",
					dlcSharedObject:   "libstyle.so",
					pluginName:        "plugin_processor_create_ast",
					blockSizeFrames:   480,
					inputWavFrameRate: 24000,
				},
				ExtraSoftwareDeps: []string{
					"amd64", // libstyle.so is amd64 only.
				},
			},
			{
				Name: "nc_sleep_10ms",
				Val: offlinePipelineBenchmarkParam{
					dlcID:             "nc-ap-dlc",
					dlcSharedObject:   "libdenoiser.so",
					pluginName:        "plugin_processor_create",
					blockSizeFrames:   480,
					inputWavFrameRate: 48000,
					sleepTime:         10 * time.Millisecond,
				},
				ExtraSoftwareDeps: []string{
					"amd64", // libdenoiser.so is amd64 only.
				},
			},
			{
				Name: "ast_sleep_20ms",
				Val: offlinePipelineBenchmarkParam{
					dlcID:             "nuance-dlc",
					dlcSharedObject:   "libstyle.so",
					pluginName:        "plugin_processor_create_ast",
					blockSizeFrames:   480,
					inputWavFrameRate: 24000,
					sleepTime:         20 * time.Millisecond,
				},
				ExtraSoftwareDeps: []string{
					"amd64", // libstyle.so is amd64 only.
				},
			},
		},
	})
}

type offlinePipelineBenchmarkParam struct {
	dlcID             string
	dlcSharedObject   string
	pluginName        string
	blockSizeFrames   int
	inputWavFrameRate int
	sleepTime         time.Duration
}

// OfflinePipelineBenchmark benchmarks audio_processor modules using offline-pipeline.
func OfflinePipelineBenchmark(ctx context.Context, s *testing.State) {
	param := s.Param().(offlinePipelineBenchmarkParam)

	if err := dlc.Install(ctx, param.dlcID, ""); err != nil {
		s.Fatalf("Failed to install DLC %q: %v", param.dlcID, err)
	}

	dlcState, err := dlc.GetDlcState(ctx, param.dlcID)
	if err != nil {
		s.Fatalf("Failed to get DLC %q state: %v", param.dlcID, err)
	}

	sharedObject := filepath.Join(dlcState.RootPath, param.dlcSharedObject)
	inputWav := filepath.Join(s.OutDir(), "input.wav")
	outputWav := filepath.Join(s.OutDir(), "output.wav")

	// Generate 60 seconds sine wave data.
	if err := testexec.CommandContext(
		ctx,
		"sox",
		"-n", "-L", "-e", "signed-integer",
		"-b", "16",
		"-r", strconv.Itoa(param.inputWavFrameRate),
		"-c", "1",
		inputWav,
		"synth", "60",
		"sine", "300",
		"gain", "-10",
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to generate test data: ", err)
	}

	stdout, err := testexec.CommandContext(
		ctx,
		"offline-pipeline", "--json",
		fmt.Sprintf("--plugin-name=%s", param.pluginName),
		fmt.Sprintf("--block-size-frames=%d", param.blockSizeFrames),
		fmt.Sprintf("--sleep-sec=%v", param.sleepTime.Seconds()),
		sharedObject, inputWav, outputWav,
	).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Command offline-pipeline failed: ", err)
	}

	type perfStats struct {
		// Use pointers to cause a runtime panic for incorrectly named fields.
		MinMs          *float64 `json:"min_ms"`
		MaxMs          *float64 `json:"max_ms"`
		MeanMs         *float64 `json:"mean_ms"`
		RealTimeFactor *float64 `json:"real_time_factor"`
	}
	var output struct {
		CPU  *perfStats `json:"cpu"`
		Wall *perfStats `json:"wall"`
	}
	if err := json.NewDecoder(bytes.NewReader(stdout)).Decode(&output); err != nil {
		s.Fatalf("Failed to decode output: %s", stdout)
	}

	p := perf.NewValues()
	for name, stats := range map[string]*perfStats{
		"cpu_time":  output.CPU,
		"wall_time": output.Wall,
	} {
		p.Set(
			perf.Metric{Name: name, Variant: "min", Unit: "ms", Direction: perf.SmallerIsBetter},
			*stats.MinMs,
		)
		p.Set(
			perf.Metric{Name: name, Variant: "max", Unit: "ms", Direction: perf.SmallerIsBetter},
			*stats.MaxMs,
		)
		p.Set(
			perf.Metric{Name: name, Variant: "mean", Unit: "ms", Direction: perf.SmallerIsBetter},
			*stats.MeanMs,
		)
		p.Set(
			perf.Metric{Name: name, Variant: "real_time_factor", Unit: "ratio", Direction: perf.SmallerIsBetter},
			*stats.RealTimeFactor,
		)
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
