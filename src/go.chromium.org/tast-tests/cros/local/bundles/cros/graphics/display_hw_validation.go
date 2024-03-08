// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/graphics"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisplayHwValidation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Performs HW Validation (i.e. PVS) for CrOS Display. The tests are handpicked for subtests that make use of the hardware rather than relying entirely on the driver implementation",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		// ChromeOS > Platform > Graphics > Display
		BugComponent: "b:188154",
		SoftwareDeps: []string{"drm_atomic", "igt", "no_qemu"},
		Fixture:      "chromeGraphicsIgt",
		Params: []testing.Param{
			{
				Name: "kms_addfb_basic",
				Val: graphics.IgtTest{
					Exe:      "kms_addfb_basic",
					Subtests: []string{"unused-modifier", "invalid-smem-bo-on-discrete", "legacy-format"},
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0004-v01", "gpu-kern-0012-v01"},
			},
			{
				Name: "kms_atomic",
				Val: graphics.IgtTest{
					Exe:      "kms_atomic",
					Subtests: []string{"plane-primary-legacy", "plane-overlay-legacy", "plane-cursor-legacy"},
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0037-v01", "gpu-kern-0002-v01", "gpu-kern-0003-v01", "gpu-kern-0006-v01", "gpu-kern-0007-v01", "gpu-kern-0009-v01"},
			},
			{
				Name: "kms_atomic_interruptible",
				Val: graphics.IgtTest{
					Exe: "kms_atomic_interruptible",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0037-v01"},
			},
			{
				Name: "kms_bw",
				Val: graphics.IgtTest{
					Exe:      "kms_bw",
					IsSkipOk: true,
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"vid-out-0001-v01", "vid-out-0002-v03"},
			},
			{
				Name: "kms_concurrent",
				Val: graphics.IgtTest{
					Exe: "kms_concurrent",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0002-v01", "gpu-kern-0044-v01"},
			},
			{
				Name: "kms_cursor_crc",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_crc",
				},
				Timeout:           15 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0038-v01"},
			},
			{
				Name: "kms_cursor_legacy",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_legacy",
					Subtests: []string{"nonblocking-modeset-vs-cursor-atomic",
						"2x-flip-vs-cursor-legacy",
						"flip-vs-cursor-crc-legacy",
						"basic-busy-flip-before-cursor-legacy"},
				},
				Timeout:           20 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0010-v01"},
			},
			{
				Name: "kms_dp_aux_dev",
				Val: graphics.IgtTest{
					Exe: "kms_dp_aux_dev",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraHardwareDeps: hwdep.D(hwdep.ExternalDisplay()),
				ExtraRequirements: []string{"gpu-kern-0006-v01"},
			},
			{
				Name: "kms_flip",
				Val: graphics.IgtTest{
					Exe:                "kms_flip",
					Subtests:           []string{"basic-plain-flip"},
					DisableSysLogCheck: true,
				},
				Timeout:           30 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0006-v01", "gpu-kern-0007-v01"},
			},
			{
				Name: "kms_invalid_mode",
				Val: graphics.IgtTest{
					Exe: "kms_invalid_mode",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"vid-out-0001-v01", "vid-out-0002-v03"},
			},
			{
				Name: "kms_panel_fitting",
				Val: graphics.IgtTest{
					Exe:      "kms_panel_fitting",
					Subtests: []string{"legacy"},
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0025-v01"},
			},
			{
				Name: "kms_pipe_crc_basic",
				Val: graphics.IgtTest{
					Exe:                "kms_pipe_crc_basic",
					DisableSysLogCheck: true,
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0038-v01"},
			},
			{
				Name: "kms_plane",
				Val: graphics.IgtTest{
					Exe:      "kms_plane",
					Subtests: []string{"pixel-format", "plane-panning-top-left", "plane-panning-bottom-right"},
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0003-v01", "gpu-kern-0038-v01"},
			},
			{
				Name: "kms_plane_alpha_blend",
				Val: graphics.IgtTest{
					Exe:      "kms_plane_alpha_blend",
					Subtests: []string{"alpha-basic"},
				},
				ExtraRequirements: []string{"gpu-kern-0024-v01"},
			},
			{
				Name: "kms_plane_cursor",
				Val: graphics.IgtTest{
					Exe: "kms_plane_cursor",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0024-v01"},
			},
			{
				Name: "kms_plane_scaling",
				Val: graphics.IgtTest{
					Exe: "kms_plane_scaling",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0003-v01", "gpu-kern-0014-v01", "gpu-kern-0039-v01", "gpu-kern-0040-v01", "gpu-kern-0041-v01"},
			},
			{
				Name: "kms_rotation_crc",
				Val: graphics.IgtTest{
					Exe: "kms_rotation_crc",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0027-v01"},
			},
			{
				Name: "kms_scaling_modes",
				Val: graphics.IgtTest{
					Exe: "kms_scaling_modes",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0025-v01"},
			},
			{
				Name: "kms_setmode",
				Val: graphics.IgtTest{
					Exe: "kms_setmode",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"vid-out-0001-v01", "vid-out-0002-v03"},
			},
			{
				Name: "kms_sysfs_edid_timing",
				Val: graphics.IgtTest{
					Exe: "kms_sysfs_edid_timing",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"disp-xface-0001-v01"},
			},
			{
				Name: "testdisplay",
				Val: graphics.IgtTest{
					Exe: "testdisplay",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"group:graphics", "graphics_igt", "graphics_weekly"},
				ExtraRequirements: []string{"vid-out-0011-v01", "vid-out-0012-v01", "gpu-kern-0006-v01"},
			},
		},
	})
}

func DisplayHwValidation(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.IgtTest)
	if testOpt.DisableSysLogCheck {
		graphics.DisableSysLogCheck(s.TestName())
	}

	f, err := os.Create(filepath.Join(s.OutDir(), filepath.Base(testOpt.Exe)+".txt"))
	if err != nil {
		s.Fatal("Failed to create a log file: ", err)
	}
	defer f.Close()

	isExitErr, exitErr, err := graphics.IgtExecuteTests(ctx, testOpt, f)

	isError, outputLog := graphics.IgtProcessResults(testOpt, f, isExitErr, exitErr, err)

	if isError {
		s.Error(outputLog)
	} else {
		s.Log(outputLog)
	}
}
