// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MIPICameraInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect camera modules info",
		Contacts:     []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera_dependent", "group:camera-stability"},
		SoftwareDeps: []string{caps.BuiltinMIPICamera},
		HardwareDeps: hwdep.D(hwdep.CameraEnumerated()),
	})
}

func MIPICameraInfo(ctx context.Context, s *testing.State) {
	mipiCams, err := testutil.MIPICamerasFromCrOSCameraTool(ctx)
	if err != nil {
		s.Fatal("Failed to get MIPI camera info: ", err)
	}

	for i, cam := range mipiCams {
		module := cam["module_id"]
		json, err := json.Marshal(cam)
		if err != nil {
			s.Fatal("Failed to re-marshal cros-camera-tool output")
		}
		if module != "" {
			filename := fmt.Sprintf("%s/module%d_%s_%s_cros-camera-tool.txt", s.OutDir(), i, module, cam["sensor_id"])
			os.WriteFile(filename, []byte(json), 0644)
		}
	}
}
