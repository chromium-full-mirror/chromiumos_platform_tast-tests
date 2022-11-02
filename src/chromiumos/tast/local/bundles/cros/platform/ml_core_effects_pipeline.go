// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/testing"
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
		BugComponent: "b:255451722",
		Attr: []string{
			"group:mainline", "informational",
		},
		SoftwareDeps: []string{"camera_feature_effects"},
		Params: []testing.Param{
			{
				Name: "effects_pipeline",
				// this binary is installed from ml-core-tests
				// into /usr/bin/
				Val: []string{"ml_core_effects_pipeline_test"},
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
