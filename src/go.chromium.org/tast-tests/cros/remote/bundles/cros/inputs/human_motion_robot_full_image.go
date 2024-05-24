// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast/core/testing"
)

const (
	simpleReferenceFileName = "human_motion_robot_full_image_simple"
)

// referenceFileData contains data associated with the input csv file used to generate the motions for a particular test.
type referenceFileData struct {
	filename string
	date     string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HumanMotionRobotFullImage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run a full image HMR Test",
		Contacts: []string{
			"chromeos-tango@google.com",
			"wmahon@google.com", // Test author
		},
		BugComponent: "b:189315", // ChromeOS > Platform > System > Input > Stylus
		Attr:         []string{"group:human_motion_robot"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.inputs.StylusEvtestCaptureService"},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name:      "simple",
			Val:       referenceFileData{filename: simpleReferenceFileName, date: "20240523"},
			ExtraData: []string{simpleReferenceFileName + ".csv"},
		}},
	})
}

func HumanMotionRobotFullImage(ctx context.Context, s *testing.State) {
	// TODO(b/343548313): Verify that a valid calibration file exists on the HMR.
	// TODO(b/343548313): Collect screen size information from the DUT and use this to verify that there is an appropriately sized gcode file on the HMR.
	// TODO(b/343548313): Run HMR job and collect touch events.
	// TODO(b/343548793): Full image analysis.
}
