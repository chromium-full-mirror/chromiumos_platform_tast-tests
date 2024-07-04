// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    CrosCameraEffectsStreamManipulator,
		Desc:    "Validates that the Effects Stream Manipulator is working with the libcros_ml_core_internal.so library from G3",
		Timeout: 2 * time.Minute,
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"cros-video-conference-tast-tests@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:1140118",
		Attr: []string{
			"group:camera_dependent",
			"group:video_conference",
			"video_conference_cq_critical",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		Fixture:      fixture.LoggedInWithFakeHALAndEffectsEnabled,
		SoftwareDeps: []string{"camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		// This binary is installed from cros-camera-effects-sm-tests
		// into /usr/local/bin/.
	})
}

func CrosCameraEffectsStreamManipulator(ctx context.Context, s *testing.State) {
	testSuiteName := "cros_effects_sm_tests"
	s.Run(ctx, "CrosEffectsSMTests", func(ctx context.Context, s *testing.State) {
		cmd := testexec.CommandContext(ctx, testSuiteName)
		if err := cmd.Run(testexec.DumpLogOnError); err != nil {
			s.Error("Failed to run test suite: ", err)
		}
	})

	s.Run(ctx, "OpenCLCacheStartup", func(ctx context.Context, s *testing.State) {
		gTestFilter := "--gtest_filter=EffectsStreamManipulatorTest.OpenCLCacheStartup"

		start := time.Now()
		cmd := testexec.CommandContext(ctx, testSuiteName, gTestFilter)
		if err := cmd.Run(testexec.DumpLogOnError); err != nil {
			s.Error("Failed to run external command: ", err)
		}
		end := time.Now()
		elapsed := end.Sub(start).Seconds()
		s.Log("OpenCLCacheStartup time: ", elapsed, " seconds")

		p := perf.NewValues()
		p.Set(perf.Metric{
			Name:      "startupTime",
			Unit:      "s",
			Direction: perf.SmallerIsBetter,
			Multiple:  false},
			float64(elapsed))

		if err := p.Save(s.OutDir()); err != nil {
			s.Error("Cannot save perf data: ", err)
		}
	})

}
