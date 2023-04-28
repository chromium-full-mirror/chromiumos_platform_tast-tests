// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"chromiumos/tast/local/graphics/expectations"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ExpectationsIdentifiers,
		Desc: "Checks expectations identity parsing capabilities on the DUT",
		Contacts: []string{
			"chromeos-gfx@google.com",
			"nhebert@chromium.org",
			"ihf@chromium.org",
		},
		// ChromeOS > Platform > Graphics
		BugComponent: "b:885255",
		// TODO(b:261224571) Once the test cases are passing on all
		// supported devices, add to group:mainline to enable CQ
		// pre-submit testing.
		Attr:    []string{"group:graphics", "graphics_perbuild"},
		Fixture: "gpuWatchHangs",
		Params: []testing.Param{
			{
				Name:    "model",
				Val:     expectations.ModelFile,
				Timeout: 30 * time.Second,
			},
			{
				Name:    "build_board",
				Val:     expectations.BuildBoardFile,
				Timeout: 30 * time.Second,
			},
			{
				Name:    "board",
				Val:     expectations.BoardFile,
				Timeout: 30 * time.Second,
			},
			{
				Name:    "gpu_chipset",
				Val:     expectations.GpuChipsetFile,
				Timeout: 30 * time.Second,
			},
			{
				Name:    "all_devices",
				Val:     expectations.AllDevicesFile,
				Timeout: 30 * time.Second,
			},
		},
	})
}

// typeSourceMap contains a mapping of file type to the underlying source.
// This is used for logging.
var typeSourceMap = map[expectations.FileType]string{
	expectations.ModelFile:      "cros_config or crossystem hwid",
	expectations.BuildBoardFile: "lsbrelease",
	expectations.BoardFile:      "lsbrelease",
	expectations.GpuChipsetFile: "hardware_probe",
	expectations.AllDevicesFile: "expectations.go",
}

// ExpectationsIdentifiers runs the expectations.FileType GetDeviceIdentifier
// method. If there is an error detected, the test case will fail with a
// message that contains the source of the identifier.
func ExpectationsIdentifiers(ctx context.Context, s *testing.State) {
	ft := s.Param().(expectations.FileType)
	id, err := ft.GetDeviceIdentifier(ctx)
	if err != nil {
		// Create a message containing the source for the file type identifier.
		var sourceMessage string
		source, ok := typeSourceMap[ft]
		if ok {
			sourceMessage = ". Is " + source + " broken on this device"
		}

		s.Fatalf("Failed to identify device by %s: %v%s", ft, err, sourceMessage)
	}
	testing.ContextLogf(ctx, "Device was identified as with type %s", id)
}
