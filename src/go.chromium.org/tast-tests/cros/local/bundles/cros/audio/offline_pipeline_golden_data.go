// Copyright 2025 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/audio/audioprocessor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/internal"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OfflinePipelineGoldenData,
		Desc:         "Verify audio_processor modules using offline-pipeline and golden data",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "hunghsienchen@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{
			"chrome", // For DLC.
		},
		Data: []string{
			data.WavesMaxxChromeConfig,
			data.WavesMaxxChromeGoldenInput,
			data.WavesMaxxChromeGoldenOutput,
		},
		Params: []testing.Param{
			{
				Name: "waves_speaker",
				Val: offlinePipelineGoldenDataParam{
					plugin: &audioprocessor.Plugin{
						// The plugin should exist in WavesOutputModels.
						Path:        "libmaxxchromeplugin.so",
						Constructor: "maxxchrome_spk_processor_create",
					},
					blockSizeFrames: 256,
					goldenInput:     data.WavesMaxxChromeGoldenInput,
					goldenOutput:    data.WavesMaxxChromeGoldenOutput,
					configFilePaths: map[string]string{
						// Waves claims that their program will look up this path
						// inside minijail0.
						data.WavesMaxxChromeConfig: "/tmp/maxxchrome/maxx_conf.ini",
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.WavesOutputModels...)),
			},
		},
	})
}

type offlinePipelineGoldenDataParam struct {
	plugin          audioprocessor.PluginInstaller
	blockSizeFrames int
	goldenInput     string
	goldenOutput    string
	configFilePaths map[string]string
}

// OfflinePipelineGoldenData uses offline-pipeline to process
// golden_input with audio_processor to generate output, and
// verify that output is exactly the same as golden_output.
func OfflinePipelineGoldenData(ctx context.Context, s *testing.State) {
	param := s.Param().(offlinePipelineGoldenDataParam)

	plugin, err := param.plugin.Install(ctx)
	if err != nil {
		s.Fatal("Failed to install plugin: ", err)
	}

	if param.goldenInput == "" || param.goldenOutput == "" {
		s.Fatal("Golden data not specified")
	}

	goldenInputPath := s.DataPath(param.goldenInput)
	goldenOutputPath := s.DataPath(param.goldenOutput)
	outputPath := filepath.Join(s.OutDir(), "output.wav")

	mountedInDir := "/tmp/offline_pipeline_golden_data_in"
	mountedInputPath := filepath.Join(mountedInDir, param.goldenInput)
	mountedOutDir := "/tmp/offline_pipeline_golden_data_out"
	mountedOutputPath := filepath.Join(mountedOutDir, "output.wav")

	args := []string{
		"-P", "/mnt/offline_pipeline_golden_data",
		"-b", "/",
		"-b", "/tmp,,1",
		// Allow executing the libraries
		"--fs-path-rx=/usr/lib64",
		// Input path, read-only
		"-b", filepath.Dir(goldenInputPath) + "," + mountedInDir,
		"--fs-path-ro=" + mountedInDir,
		// Output path
		"-b", s.OutDir() + "," + mountedOutDir + ",1",
		"--fs-path-rw=" + mountedOutDir,
	}

	for path, mountedPath := range param.configFilePaths {
		args = append(args,
			"-b", s.DataPath(path)+","+mountedPath,
			"--fs-path-ro="+mountedPath,
		)
	}

	args = append(args,
		"--", "/usr/local/bin/offline-pipeline",
		fmt.Sprintf("--plugin-name=%s", plugin.Constructor),
		fmt.Sprintf("--block-size-frames=%d", param.blockSizeFrames),
		plugin.Path, mountedInputPath, mountedOutputPath,
	)

	if err = testexec.CommandContext(
		ctx, "/sbin/minijail0", args...,
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Command offline-pipeline (in minijail0) failed: ", err)
	}

	if err = audio.CheckWavsSame(ctx, outputPath, goldenOutputPath); err != nil {
		s.Fatal("CheckWavsSame failed: ", err)
	}
}
