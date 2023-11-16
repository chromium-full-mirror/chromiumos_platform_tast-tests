// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    MlCoreEffectsPipeline,
		Desc:    "Validates that the ML Core bindings layer interacts correctly with the libcros_ml_core_internal.so library from G3",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
		},
		BugComponent: "b:1140118",
		Attr: []string{
			"group:camera_dependent",
		},
		Fixture:      fixture.LoggedInWithFakeHALAndEffectsEnabled,
		SoftwareDeps: []string{"camera_feature_effects"},
		Params: []testing.Param{
			{
				Name: "effects_pipeline",
				// this binary is installed from ml-core-tests
				// into /usr/bin/
				ExtraAttr: []string{
					"group:video_conference",
					"group:cbx", "cbx_feature_enabled", "cbx_unstable",
				},
				ExtraTestBedDeps:  []string{tbdep.Cbx(false)},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
				Val:               []string{"ml_core_effects_pipeline_test"},
			},
			{
				Name: "effects_pipeline_betty",
				// this binary is installed from ml-core-tests
				// into /usr/bin/
				ExtraHardwareDeps: hwdep.D(hwdep.Model("betty")),
				Val:               []string{"ml_core_effects_pipeline_test", "--use_opengl"},
			},
		},
	})
}

func MlCoreEffectsPipeline(ctx context.Context, s *testing.State) {
	cmdArgs := s.Param().([]string)

	cmd := testexec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Error("Failed to run test suite: ", err)
	}
}
