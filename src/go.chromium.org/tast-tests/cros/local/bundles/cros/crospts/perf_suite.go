// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crospts

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/crospts/ptsworld"
	"go.chromium.org/tast/core/testing"
)

type perfSuite struct {
	runner    ptsworld.Runner
	suiteName string
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
				runner:    ptsworld.NewCrosRunner(),
				suiteName: "cros-core-performance",
			},
			// TODO(darrenwu): The test time was tested on brya. Need to run the
			// test on other low end DUT.
			Timeout: 4 * time.Hour,
		}, {
			Name:    "all_cros_arm64",
			Fixture: "mountUnmountPtsWorldForCrOSarm64",
			Val: &perfSuite{
				runner:    ptsworld.NewCrosRunner(),
				suiteName: "cros-core-performance",
			},
			// TODO(darrenwu): The test time was tested on cherry. Need to run the
			// test on other low end DUT.
			Timeout: 4 * time.Hour,
		}},
	})
}

// PerfSuite runs the performance test suite.
func PerfSuite(ctx context.Context, s *testing.State) {
	perfSuite := s.Param().(*perfSuite)
	s.Logf("Running perf test suite: %s", perfSuite.suiteName)
	perfSuite.runner.RunTestSuite(ctx, s, perfSuite.suiteName)
}
