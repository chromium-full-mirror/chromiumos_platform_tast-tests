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
		Desc: "Run the full test of cros-core-performance suite",
		Contacts: []string{
			"cros-core-systems-perf@google.com",
			"darrenwu@google.com",
		},
		BugComponent: "b:167279", // ChromeOS > Software > baseOS > Performance
		Params: []testing.Param{{
			Name:    "all_cros_x86",
			Fixture: "mountUnmountPtsWorldForCrOSx86",
			Val: &perfSuite{
				runner:        ptsworld.NewCrosRunner(),
				suiteName:     "cros-core-performance",
				resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
			},
			// TODO(darrenwu): The test time was tested on brya. Need to run the
			// test on other low end DUT.
			Timeout: 4 * time.Hour,
		}, {
			Name:    "all_cros_arm64",
			Fixture: "mountUnmountPtsWorldForCrOSarm64",
			Val: &perfSuite{
				runner:        ptsworld.NewCrosRunner(),
				suiteName:     "cros-core-performance",
				resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
			},
			// TODO(darrenwu): The test time was tested on cherry. Need to run the
			// test on other low end DUT.
			Timeout: 4 * time.Hour,
		}, {
			Name:    "leveldb_cros_x86",
			Fixture: "mountUnmountPtsWorldForCrOSx86",
			Val: &perfSuite{
				runner:        ptsworld.NewCrosRunner(),
				suiteName:     "leveldb-1.0.2",
				resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
			},
			// TODO(darrenwu): The test time was tested on brya. Need to run the
			// test on other low end DUT.
			Timeout: 40 * time.Minute,
		}, {
			Name:    "leveldb_cros_arm64",
			Fixture: "mountUnmountPtsWorldForCrOSarm64",
			Val: &perfSuite{
				runner:        ptsworld.NewCrosRunner(),
				suiteName:     "leveldb-1.0.2",
				resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
			},
			// TODO(darrenwu): The test time was tested on cherry. Need to run the
			// test on other low end DUT.
			Timeout: 1 * time.Hour,
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
				suiteName:     "cachebench-1.1.2",
				resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
			},
			Timeout: 1 * time.Hour,
		}, {
			Name:    "cachebench_cros_arm64",
			Fixture: "mountUnmountPtsWorldForCrOSarm64",
			Val: &perfSuite{
				runner:        ptsworld.NewCrosRunner(),
				suiteName:     "cachebench-1.1.2",
				resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
			},
			// TODO(darrenwu): The test time was tested on cherry. Need to run the
			// test on other low end DUT.
			Timeout: 1 * time.Hour,
		}, {
			Name:    "compress7zip_cros_x86",
			Fixture: "mountUnmountPtsWorldForCrOSx86",
			Val: &perfSuite{
				runner:        ptsworld.NewCrosRunner(),
				suiteName:     "compress-7zip-1.10.0",
				resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
			},
			Timeout: 10 * time.Minute,
		}, {
			Name:    "compress7zip_cros_arm64",
			Fixture: "mountUnmountPtsWorldForCrOSarm64",
			Val: &perfSuite{
				runner:        ptsworld.NewCrosRunner(),
				suiteName:     "compress-7zip-1.10.0",
				resultsParser: metrics.NewResultsParser(ptsworld.CrosResultsDir, ptsworld.TypeCros),
			},
			Timeout: 10 * time.Minute,
		},
		},
	})
}

// PerfSuite runs the performance test suite.
func PerfSuite(ctx context.Context, s *testing.State) {
	perfSuite := s.Param().(*perfSuite)
	s.Logf("Running perf test suite: %s", perfSuite.suiteName)
	perfSuite.runner.RunTestSuite(ctx, s, perfSuite.suiteName)
	err := perfSuite.resultsParser.ConvertMetrics(s.OutDir())
	if err != nil {
		s.Error("Failed to convert metrics: ", err)
	}
}
