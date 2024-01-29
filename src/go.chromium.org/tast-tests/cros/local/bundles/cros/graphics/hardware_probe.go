// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/graphics/hardwareprobe"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: HardwareProbe,
		Desc: "Verify hardware_probe binary can detect various information",
		Contacts: []string{
			"chromeos-gfx@google.com",
			"pwang@chromium.org",
		},
		BugComponent: "b:995569", // ChromeOS > Platform > Graphics > GPU
		Attr:         []string{"group:graphics", "graphics_perbuild", "group:mainline"},
		Fixture:      "gpuWatchDog",
		Params: []testing.Param{{
			Val:       false,
			ExtraAttr: []string{"group:cq-medium", "group:crosbolt", "crosbolt_fsi_check"},
		}, {
			Name:      "verify",
			Val:       true,
			ExtraAttr: []string{"group:cq-medium", "group:crosbolt", "crosbolt_fsi_check"},
		}},
	})
}

// HardwareProbe verifies we can successfully retrieve various device information via hardware_probe.
func HardwareProbe(ctx context.Context, s *testing.State) {
	result, err := hardwareprobe.GetHardwareProbeResult(ctx)
	if err != nil {
		s.Fatal("Failed to run hardware_probe: ", err)
	}

	s.Log("Successfully get the information: ", result)
	for _, info := range result.GPUInfo {
		s.Log("GPU_Family: ", info.Family)
		s.Log("GPU_Vendor: ", info.Vendor)
	}
	s.Log("CPU_Family: ", result.CPUFamily)

	check := s.Param().(bool)
	if !check {
		return
	}
	// Check if fields are valid.
	if result.GPUInfo == nil || len(result.GPUInfo) == 0 {
		s.Error("Failed to find any GPU in hardware_probe result")
	}
	for _, info := range result.GPUInfo {
		if info.Family == "unknown" || info.Family == "" {
			s.Error("Unrecognized gpu family: ", info.Family)
		}
		if info.Vendor == "unknown" || info.Vendor == "" {
			s.Error("Unrecognized gpu Vendor: ", info.Vendor)
		}
	}
	if result.CPUFamily == "unknown" || result.CPUFamily == "" {
		s.Error("Unrecognized CPU family: ", result.CPUFamily)
	}
	if result.Disk == nil {
		s.Error("Failed to get disk information")
	}
	if result.Memory == 0 {
		s.Error("Failed to get memory size")
	}
}
