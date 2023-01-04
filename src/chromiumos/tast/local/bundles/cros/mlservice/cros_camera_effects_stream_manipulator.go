// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mlservice

import (
	"context"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/bundles/cros/mlservice/fixture"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    CrosCameraEffectsStreamManipulator,
		Desc:    "Validates that the Effects Stream Manipulator is working with the libcros_ml_core_internal.so library from G3",
		Timeout: 2 * time.Minute,
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shafron@google.com",
		},
		BugComponent: "b:1140118",
		Attr:         []string{"group:ml_service"},
		Fixture:      fixture.NoLoggedIn,
		SoftwareDeps: []string{"camera_feature_effects"},
		Params: []testing.Param{
			{
				Name: "effects_stream_manipulator",
				// This binary is installed from cros-camera-effects-sm-tests
				// into /usr/local/bin/.
				ExtraAttr: []string{
					"group:mainline", "informational",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
				Val:               []string{"cros_effects_sm_tests"},
			},
		},
	})
}

func CrosCameraEffectsStreamManipulator(ctx context.Context, s *testing.State) {
	cmdArgs := s.Param().([]string)

	cmd := testexec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Error("Failed to run test suite: ", err)
	}
}
