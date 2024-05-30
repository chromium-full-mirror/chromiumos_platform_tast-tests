// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crospts

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/crospts/ptsworld"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/crospts/ptsworld/metrics"
	"go.chromium.org/tast/core/testing"
)

type perfSuite struct {
	runner        ptsworld.Runner
	suiteName     string
	resultsParser *metrics.ResultsParser
}

func init() {
	testing.AddTest(&testing.Test{
		Func: PerfSuite,
		Desc: "Run the crospts microbenchmark suites",
		Contacts: []string{
			"cros-core-systems-perf@google.com",
			"darrenwu@google.com",
		},
		BugComponent: "b:167279", // ChromeOS > Software > baseOS > Performance
		Attr:         []string{"group:crospts"},
		Params: []testing.Param{
			{
				Name:    "leveldb_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "local/leveldb-1.1.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 20 * time.Minute,
			}, {
				Name:    "leveldb_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "local/leveldb-1.1.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 80 * time.Minute,
			}, {
				// TODO(b/315897893): The mbw test cannot finish in 4 hours on octopus.
				Name:    "mbw_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "mbw-1.0.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 4 * time.Hour,
			}, {
				Name:    "mbw_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "mbw-1.0.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				// TODO(darrenwu): The test time was tested on cherry. Need to run the
				// test on other low end DUT.
				Timeout: 4 * time.Hour,
			}, {
				Name:    "cachebench_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "local/cachebench-1.2.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 25 * time.Minute,
			}, {
				Name:    "cachebench_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "local/cachebench-1.2.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 25 * time.Minute,
			}, {
				Name:    "compress7zip_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "compress-7zip-1.10.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				ExtraAttr: []string{"crospts_x86"},
				Timeout:   30 * time.Minute,
			}, {
				Name:    "compress7zip_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "compress-7zip-1.10.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				ExtraAttr: []string{"crospts_arm64"},
				Timeout:   30 * time.Minute,
			}, {
				Name:    "openssl_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "openssl-3.1.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 90 * time.Minute,
			}, {
				Name:    "openssl_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "openssl-3.1.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 90 * time.Minute,
			}, {
				Name:    "tjbench_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "tjbench-1.2.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				ExtraAttr: []string{"crospts_x86"},
				Timeout:   10 * time.Minute,
			}, {
				Name:    "tjbench_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "tjbench-1.2.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				ExtraAttr: []string{"crospts_arm64"},
				Timeout:   10 * time.Minute,
			}, {
				// TODO(b/316035777): The compress lz4 benchmark cannot finish test in 1 hour on octopus
				Name:    "compresslz4_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "compress-lz4-1.0.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 30 * time.Minute,
			}, {
				Name:    "compresslz4_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "compress-lz4-1.0.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 30 * time.Minute,
			}, {
				Name:    "encodemp3_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "encode-mp3-1.7.4",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 10 * time.Minute,
			}, {
				Name:    "encodemp3_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "encode-mp3-1.7.4",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 10 * time.Minute,
			}, {
				Name:    "vpxenc_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "vpxenc-3.2.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 60 * time.Minute,
			}, {
				Name:    "vpxenc_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "vpxenc-3.2.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 60 * time.Minute,
			}, {
				Name:    "tensorflowlite_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "tensorflow-lite-1.1.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 120 * time.Minute,
			}, {
				Name:    "tensorflowlite_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "tensorflow-lite-1.1.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 120 * time.Minute,
			}, {
				Name:    "rnnoise_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "rnnoise-1.0.2",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 30 * time.Minute,
			}, {
				Name:    "rnnoise_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "rnnoise-1.0.2",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 30 * time.Minute,
			}, {
				Name:    "cythonbench_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "cython-bench-1.1.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				ExtraAttr: []string{"crospts_x86"},
				Timeout:   30 * time.Minute,
			}, {
				Name:    "cythonbench_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "cython-bench-1.1.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				ExtraAttr: []string{"crospts_arm64"},
				Timeout:   30 * time.Minute,
			}, {
				Name:    "pyperf_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "pyperformance-1.0.2",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 90 * time.Minute,
			}, {
				Name:    "pyperf_cros_arm64",
				Fixture: "mountUnmountPtsWorldForCrOSarm64",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "pyperformance-1.0.2",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 90 * time.Minute,
			}, {
				Name:    "ctxclock_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "ctx-clock-1.0.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 15 * time.Minute,
			}, {
				Name:    "mutex_cros_x86",
				Fixture: "mountUnmountPtsWorldForCrOSx86",
				Val: &perfSuite{
					runner:        ptsworld.NewCrosRunner(),
					suiteName:     "mutex-1.0.0",
					resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
				},
				Timeout: 90 * time.Minute,
			},
		},
	})
}

// PerfSuite runs the performance test suite.
func PerfSuite(ctx context.Context, s *testing.State) {
	perfSuite := s.Param().(*perfSuite)
	s.Logf("Running perf test suite: %s", perfSuite.suiteName)
	perfSuite.runner.RunTestSuite(ctx, s, perfSuite.suiteName)
	err := perfSuite.resultsParser.ConvertMetrics(perfSuite.suiteName, s.OutDir())
	if err != nil {
		s.Error("Failed to convert metrics: ", err)
	}
	err = perfSuite.resultsParser.SaveArtifacts(s.OutDir())
	if err != nil {
		s.Error("Failed to save artifacts: ", err)
	}
}
