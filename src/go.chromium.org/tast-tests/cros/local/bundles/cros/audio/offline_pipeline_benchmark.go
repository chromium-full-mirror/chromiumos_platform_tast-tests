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
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio/audioprocessor"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OfflinePipelineBenchmark,
		Desc:         "Benchmark audio_processor modules using offline-pipeline",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "cychiang@chromium.org"},
		BugComponent: "b:776546",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Timeout:      8 * time.Minute,
		SoftwareDeps: []string{
			"chrome", // For DLC.
		},
		Params: []testing.Param{
			{
				Name: "nc",
				Val: offlinePipelineBenchmarkParam{
					plugin: &audioprocessor.DLCPlugin{
						DLCID:       audioprocessor.Const("nc-ap-dlc"),
						Path:        audioprocessor.Const("libeffects.so"),
						Constructor: "plugin_processor_create_nc",
					},
					blockSizeFrames:   480,
					inputWavFrameRate: 48000,
					inputWavChannels:  1,
				},
			},
			{
				Name: "ast",
				Val: offlinePipelineBenchmarkParam{
					plugin: &audioprocessor.DLCPlugin{
						DLCID:       audioprocessor.Const("nc-ap-dlc"),
						Path:        audioprocessor.Const("libeffects.so"),
						Constructor: "plugin_processor_create_ast",
					},
					blockSizeFrames:   480,
					inputWavFrameRate: 24000,
					inputWavChannels:  1,
				},
			},
			{
				Name: "bf",
				Val: offlinePipelineBenchmarkParam{
					plugin: &audioprocessor.DLCPlugin{
						DLCID:       audioprocessor.Var("beamforming_dlc_id"),
						Path:        audioprocessor.Var("beamforming_dlc_path"),
						Constructor: "plugin_processor_create",
					},
					blockSizeFrames:   256,
					inputWavFrameRate: 16000,
					inputWavChannels:  3,
				},
				ExtraTestBedDeps: []string{tbdep.AudioBeamforming("intelligo")},
			},
			{
				Name: "nc_sleep_rt",
				Val: offlinePipelineBenchmarkParam{
					plugin: &audioprocessor.DLCPlugin{
						DLCID:       audioprocessor.Const("nc-ap-dlc"),
						Path:        audioprocessor.Const("libeffects.so"),
						Constructor: "plugin_processor_create_nc",
					},
					blockSizeFrames:   480,
					inputWavFrameRate: 48000,
					inputWavChannels:  1,
					sleepTime:         10 * time.Millisecond,
					setThreadPriority: true,
				},
			},
			{
				Name: "ast_sleep_rt",
				Val: offlinePipelineBenchmarkParam{
					plugin: &audioprocessor.DLCPlugin{
						DLCID:       audioprocessor.Const("nc-ap-dlc"),
						Path:        audioprocessor.Const("libeffects.so"),
						Constructor: "plugin_processor_create_ast",
					},
					blockSizeFrames:   480,
					inputWavFrameRate: 24000,
					inputWavChannels:  1,
					sleepTime:         20 * time.Millisecond,
					setThreadPriority: true,
				},
			},
			{
				Name: "bf_sleep_rt",
				Val: offlinePipelineBenchmarkParam{
					plugin: &audioprocessor.DLCPlugin{
						DLCID:       audioprocessor.Var("beamforming_dlc_id"),
						Path:        audioprocessor.Var("beamforming_dlc_path"),
						Constructor: "plugin_processor_create",
					},
					blockSizeFrames:   256,
					inputWavFrameRate: 16000,
					inputWavChannels:  3,
					sleepTime:         16 * time.Millisecond,
					setThreadPriority: true,
				},
				ExtraTestBedDeps: []string{tbdep.AudioBeamforming("intelligo")},
			},
		},
	})
}

type offlinePipelineBenchmarkParam struct {
	plugin            *audioprocessor.DLCPlugin
	blockSizeFrames   int
	inputWavFrameRate int
	inputWavChannels  int
	sleepTime         time.Duration
	setThreadPriority bool
}

// OfflinePipelineBenchmark benchmarks audio_processor modules using offline-pipeline.
func OfflinePipelineBenchmark(ctx context.Context, s *testing.State) {
	param := s.Param().(offlinePipelineBenchmarkParam)

	plugin, err := param.plugin.Install(ctx)
	if err != nil {
		s.Fatal("Failed to install plugin: ", err)
	}

	inputWav := filepath.Join(s.OutDir(), "input.wav")
	outputWav := filepath.Join(s.OutDir(), "output.wav")

	// Generate 60 seconds sine wave data.
	if err := testexec.CommandContext(
		ctx,
		"sox",
		"-n", "-L", "-e", "signed-integer",
		"-b", "16",
		"-r", strconv.Itoa(param.inputWavFrameRate),
		"-c", strconv.Itoa(param.inputWavChannels),
		inputWav,
		"synth", "60",
		"sine", "300",
		"gain", "-10",
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to generate test data: ", err)
	}

	cmd := testexec.CommandContext(
		ctx,
		"offline-pipeline", "--json",
		fmt.Sprintf("--plugin-name=%s", plugin.Constructor),
		fmt.Sprintf("--block-size-frames=%d", param.blockSizeFrames),
		plugin.Path, inputWav, outputWav,
	)
	if param.sleepTime != 0 {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--sleep-sec=%v", param.sleepTime.Seconds()))
	}
	if param.setThreadPriority {
		cmd.Args = append(cmd.Args, "--set-thread-priority")
	}
	stdout, err := cmd.Output(testexec.DumpLogOnError)
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
		CPU      *perfStats `json:"cpu"`
		Wall     *perfStats `json:"wall"`
		MaxRSSKB *int       `json:"max_rss_kb"`
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
	p.Set(
		perf.Metric{Name: "rss", Variant: "max", Unit: "bytes", Direction: perf.SmallerIsBetter},
		float64(*output.MaxRSSKB)*1024,
	)
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
