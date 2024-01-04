// Copyright 2020 The ChromiumOS Authors
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
		Func: IgtKms,
		Desc: "IGT KMS Tests that we care about the most. This list also includes all tests in PVS and PE",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"ddavenport@chromium.org",
			"markyacoub@google.com",
		},
		// ChromeOS > Platform > Graphics > Display
		BugComponent: "b:188154",
		SoftwareDeps: []string{"drm_atomic", "igt", "no_qemu"},
		Attr:         []string{"group:graphics", "graphics_igt"},
		Fixture:      "chromeGraphicsIgt",
		Params: []testing.Param{
			{
				Name: "drm_mm",
				Val: graphics.IgtTest{
					Exe: "drm_mm",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "drm_read",
				Val: graphics.IgtTest{
					Exe: "drm_read",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_addfb_basic",
				Val: graphics.IgtTest{
					Exe: "kms_addfb_basic",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform("grunt", "zork", "guybrush")),
			}, {
				Name: "kms_atomic",
				Val: graphics.IgtTest{
					Exe: "kms_atomic",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_perbuild"},
			}, {
				Name: "kms_atomic_interruptible",
				Val: graphics.IgtTest{
					Exe: "kms_atomic_interruptible",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_perbuild"},
			}, {
				Name: "kms_atomic_transition",
				Val: graphics.IgtTest{
					Exe: "kms_atomic_transition",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform("grunt", "zork", "guybrush")),
			}, {
				Name: "kms_async_flips",
				Val: graphics.IgtTest{
					Exe: "kms_async_flips",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(graphics.IgtGpuQcom...)),
			}, {
				Name: "kms_busy",
				Val: graphics.IgtTest{
					Exe:                "kms_busy",
					DisableSysLogCheck: true,
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_bw",
				Val: graphics.IgtTest{
					Exe: "kms_bw",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_color",
				Val: graphics.IgtTest{
					Exe: "kms_color",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(graphics.IgtGpuMtk...)),
			}, {
				Name: "kms_color_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_color",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform(graphics.IgtGpuMtk...)),
			}, {
				Name: "kms_concurrent",
				Val: graphics.IgtTest{
					Exe: "kms_concurrent",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(graphics.IgtGpuMtk...)),
			}, {
				Name: "kms_concurrent_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_concurrent",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform(graphics.IgtGpuMtk...)),
			}, {
				Name: "kms_content_protection",
				Val: graphics.IgtTest{
					Exe: "kms_concurrent",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_cursor_crc",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_crc",
				},
				Timeout:           15 * time.Minute,
				ExtraAttr:         []string{"graphics_weekly"},
				ExtraSoftwareDeps: []string{"no_amd_cpu"},
			}, {
				Name: "kms_cursor_crc_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_crc",
				},
				Timeout:           15 * time.Minute,
				ExtraAttr:         []string{"graphics_weekly"},
				ExtraSoftwareDeps: []string{"amd_cpu"},
			}, {
				Name: "kms_cursor_edge_walk",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_edge_walk",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_cursor_legacy",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_legacy",
				},
				Timeout:           20 * time.Minute,
				ExtraAttr:         []string{"graphics_weekly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(graphics.IgtGpuQcom...)),
			}, {
				Name: "kms_cursor_legacy_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_legacy",
				},
				Timeout:           20 * time.Minute,
				ExtraAttr:         []string{"graphics_weekly"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform(graphics.IgtGpuQcom...)),
			}, {
				Name: "kms_dp_aux_dev",
				Val: graphics.IgtTest{
					Exe: "kms_dp_aux_dev",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraSoftwareDeps: []string{"no_amd_cpu"},
			}, {
				Name: "kms_flip",
				Val: graphics.IgtTest{
					Exe:                "kms_flip",
					DisableSysLogCheck: true,
				},
				Timeout:   30 * time.Minute,
				ExtraAttr: []string{"graphics_weekly"},
				ExtraHardwareDeps: hwdep.D(
					hwdep.SkipOnPlatform(graphics.IgtGpuQcom...),
					hwdep.SkipOnPlatform(graphics.IgtGpuMtk...),
				),
			}, {
				Name: "kms_flip_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_flip",
				},
				Timeout:           30 * time.Minute,
				ExtraAttr:         []string{"graphics_weekly"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform(append(graphics.IgtGpuQcom, graphics.IgtGpuMtk...)...)),
			}, {
				Name: "kms_flip_event_leak",
				Val: graphics.IgtTest{
					Exe: "kms_flip_event_leak",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_force_connector_basic",
				Val: graphics.IgtTest{
					Exe: "kms_force_connector_basic",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.HdmiConnected()),
			}, {
				Name: "kms_frontbuffer_tracking",
				Val: graphics.IgtTest{
					Exe: "kms_frontbuffer_tracking",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_getfb",
				Val: graphics.IgtTest{
					Exe: "kms_getfb",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_hdmi_inject",
				Val: graphics.IgtTest{
					Exe: "kms_hdmi_inject",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.HdmiConnected()),
			}, {
				Name: "kms_hdr",
				Val: graphics.IgtTest{
					Exe:      "kms_hdr",
					IsSkipOk: true,
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraSoftwareDeps: []string{"no_amd_cpu"},
			}, {
				Name: "kms_invalid_mode",
				Val: graphics.IgtTest{
					Exe: "kms_invalid_mode",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_weekly"},
			}, {
				Name: "kms_panel_fitting",
				Val: graphics.IgtTest{
					Exe: "kms_panel_fitting",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(
					hwdep.SkipOnPlatform(graphics.IgtGpuQcom...),
					hwdep.SkipOnPlatform(graphics.IgtGpuMtk...),
					hwdep.SkipOnPlatform("zork", "grunt"),
				),
			}, {
				Name: "kms_pipe_crc_basic",
				Val: graphics.IgtTest{
					Exe:                "kms_pipe_crc_basic",
					DisableSysLogCheck: true,
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_plane",
				Val: graphics.IgtTest{
					Exe: "kms_plane",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(graphics.IgtGpuQcom...)),
				ExtraSoftwareDeps: []string{"no_amd_cpu"},
			}, {
				Name: "kms_plane_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_plane",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform(graphics.IgtGpuQcom...)),
				ExtraSoftwareDeps: []string{"amd_cpu"},
			}, {
				Name: "kms_plane_alpha_blend",
				Val: graphics.IgtTest{
					Exe: "kms_plane_alpha_blend",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(
					hwdep.SkipOnPlatform(graphics.IgtGpuQcom...),
					hwdep.SkipOnPlatform("grunt"),
				),
			}, {
				Name: "kms_plane_alpha_blend_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_plane_alpha_blend",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform(graphics.IgtGpuQcom...)),
			}, {
				Name: "kms_plane_cursor",
				Val: graphics.IgtTest{
					Exe: "kms_plane_cursor",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(graphics.IgtGpuMtk...)),
				ExtraSoftwareDeps: []string{"no_amd_cpu"},
			}, {
				Name: "kms_plane_cursor_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_plane_cursor",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform(graphics.IgtGpuMtk...)),
				ExtraSoftwareDeps: []string{"amd_cpu"},
			}, {
				Name: "kms_plane_lowres",
				Val: graphics.IgtTest{
					Exe:      "kms_plane_lowres",
					IsSkipOk: true,
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_plane_scaling",
				Val: graphics.IgtTest{
					Exe: "kms_plane_scaling",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraSoftwareDeps: []string{"no_amd_cpu"},
			}, {
				Name: "kms_plane_scaling_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_plane_scaling",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraSoftwareDeps: []string{"amd_cpu"},
			}, {
				Name: "kms_prop_blob",
				Val: graphics.IgtTest{
					Exe: "kms_prop_blob",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_properties",
				Val: graphics.IgtTest{
					Exe: "kms_properties",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(graphics.IgtGpuMtk...)),
			}, {
				Name: "kms_properties_unstable",
				Val: graphics.IgtTest{
					Exe: "kms_properties",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform(graphics.IgtGpuMtk...)),
			}, {
				Name: "kms_psr",
				Val: graphics.IgtTest{
					Exe:      "kms_psr",
					IsSkipOk: true,
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_rmfb",
				Val: graphics.IgtTest{
					Exe: "kms_rmfb",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_rotation_crc",
				Val: graphics.IgtTest{
					Exe: "kms_rotation_crc",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_scaling_modes",
				Val: graphics.IgtTest{
					Exe: "kms_scaling_modes",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_weekly"},
				ExtraRequirements: []string{"gpu-kern-0025-v01"},
			}, {
				Name: "kms_setmode",
				Val: graphics.IgtTest{
					Exe: "kms_setmode",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_sysfs_edid_timing",
				Val: graphics.IgtTest{
					Exe: "kms_sysfs_edid_timing",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_universal_plane",
				Val: graphics.IgtTest{
					Exe: "kms_universal_plane",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "kms_vblank",
				Val: graphics.IgtTest{
					Exe:                "kms_vblank",
					DisableSysLogCheck: true,
				},
				Timeout:   15 * time.Minute,
				ExtraAttr: []string{"graphics_weekly"},
			}, {
				Name: "kms_vrr",
				Val: graphics.IgtTest{
					Exe: "kms_vrr",
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.VRR()),
			}, {
				Name: "sw_sync",
				Val: graphics.IgtTest{
					Exe: "sw_sync",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "testdisplay",
				Val: graphics.IgtTest{
					Exe: "testdisplay",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			}, {
				Name: "template",
				Val: graphics.IgtTest{
					Exe: "template",
				},
				Timeout:   1 * time.Minute,
				ExtraAttr: []string{"group:mainline", "group:cq-medium"},
			}},
	})
}

func IgtKms(ctx context.Context, s *testing.State) {
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
