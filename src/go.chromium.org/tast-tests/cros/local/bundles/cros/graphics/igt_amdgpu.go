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
)

func init() {
	testing.AddTest(&testing.Test{
		Func: IgtAmdgpu,
		Desc: "Verifies IGT amdgpu-only test binaries run successfully",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"ddavenport@chromium.org",
			"markyacoub@google.com",
		},
		// ChromeOS > Platform > Graphics > Display
		BugComponent: "b:188154",
		SoftwareDeps: []string{"drm_atomic", "igt", "no_qemu", "amd_cpu"},
		Attr:         []string{"group:graphics", "graphics_igt"},
		Fixture:      "chromeGraphicsIgt",
		Params: []testing.Param{
			{
				Name: "amd_abm",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_abm",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_assr",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_assr",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_basic",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_basic",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_bypass",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_bypass",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_color",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_color",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_cp_dma_misc",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_cp_dma_misc",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_cs_nop",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_cs_nop",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_deadlock",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_deadlock",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_dp_dsc",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_dp_dsc",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_hotplug",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_hotplug",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_ilr",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_ilr",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_info",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_info",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_link_settings",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_link_settings",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_max_bpc",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_max_bpc",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_mem_leak",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_mem_leak",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_mode_switch",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_mode_switch",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_module_load",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_module_load",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_pci_unplug",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_pci_unplug",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_plane",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_plane",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_prime",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_prime",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_psr",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_psr",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "amd_vrr_range",
				Val: graphics.IgtTest{
					Exe: "amdgpu/amd_vrr_range",
				},
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
		},
	})
}

func IgtAmdgpu(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.IgtTest)

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
