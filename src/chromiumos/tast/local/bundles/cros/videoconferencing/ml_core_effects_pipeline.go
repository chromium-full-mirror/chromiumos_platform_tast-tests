// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/videoconferencing/fixture"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    MlCoreEffectsPipeline,
		Desc:    "Validates that the ML Core bindings layer interacts correctly with the libcros_ml_core_internal.so library from G3",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shafron@google.com",
		},
		BugComponent: "b:1140118",
		Attr:         []string{"group:ml_service"},
		Fixture:      fixture.NoLogInWithInternalCameraAndEffectsEnabled,
		SoftwareDeps: []string{"camera_feature_effects"},
		Params: []testing.Param{
			{
				Name: "effects_pipeline",
				// this binary is installed from ml-core-tests
				// into /usr/bin/
				ExtraAttr: []string{
					"group:video_conference", "video_conference_cq_critical",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
				Val:               []string{"ml_core_effects_pipeline_test"},
			},
			{
				Name: "effects_pipeline_betty",
				// this binary is installed from ml-core-tests
				// into /usr/bin/
				ExtraHardwareDeps: hwdep.D(hwdep.Model("betty")),
				Val:               []string{"ml_core_effects_pipeline_test"},
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
