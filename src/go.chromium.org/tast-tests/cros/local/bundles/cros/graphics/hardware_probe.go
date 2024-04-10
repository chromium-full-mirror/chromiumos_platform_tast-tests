// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/graphics/hardwareprobe"
	"go.chromium.org/tast/core/errors"
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
		Fixture:      "gpuWatchHangs",
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
	// TODO(b:296993394): re-enable the check once IOMMU errors is fixed.
	graphics.DisableSysLogCheck(s.TestName(), graphics.SysLogMediatekIOMMUErrors)

	result, err := hardwareprobe.GetHardwareProbeResult(ctx)
	if err != nil {
		s.Fatal("Failed to run hardware_probe: ", err)
	}

	s.Log("Successfully got the information: ", result)
	for _, info := range result.GPUInfo {
		s.Log("GPU_Family: ", info.Family)
		s.Log("GPU_Vendor: ", info.Vendor)
		s.Log("GPU_ID: ", info.ID)
	}
	s.Log("CPU_Family: ", result.CPUFamily)

	check := s.Param().(bool)
	if check {
		if err := checkValueIsSet(ctx, result); err != nil {
			s.Fatal("Failed to verify values in hardware_probe result: ", err)
		}
	}
	return
}

func checkValueIsSet(ctx context.Context, result hardwareprobe.Result) error {
	var resultErr error

	// Check if fields are valid.
	if result.GPUInfo == nil || len(result.GPUInfo) == 0 {
		resultErr = errors.Wrap(resultErr, "failed to find any GPU in hardware_probe result")
	}
	for _, info := range result.GPUInfo {
		if info.Family == "unknown" || info.Family == "" {
			resultErr = errors.Wrapf(resultErr, "unrecognized GPU family %v", info.Family)
		}
		if info.Vendor == "unknown" || info.Vendor == "" {
			resultErr = errors.Wrapf(resultErr, "unrecognized GPU vendor %v", info.Vendor)
		}
		// ID should be something like intel:9b41, mediatek:mali-g52, or qualcomm:sc7180.
		if !strings.Contains(info.ID, ":") {
			resultErr = errors.Wrapf(resultErr, "unrecognized GPU id %v", info.ID)
		}
	}
	if result.CPUFamily == "unknown" || result.CPUFamily == "" {
		resultErr = errors.Wrapf(resultErr, "unrecognized CPU family %v", result.CPUFamily)
	}
	if result.Disk == nil {
		resultErr = errors.Wrap(resultErr, "failed to get platform disk information")
	}
	if result.Memory == 0 {
		resultErr = errors.Wrap(resultErr, "failed to get platform memory size")
	}

	// Check the labels reporting exists for infra.
	for _, s := range []struct {
		name     string // Name of the labels.
		required bool
	}{
		// Display properties are optional as some DUT have no display info.
		{"display_panel_name", false},
		{"display_present_hdr", false},
		{"display_present_psr", false},
		{"display_present_vrr", false},
		{"display_refresh_rate", false},
		{"display_resolution", false},
		{"gpu_family", true},
		{"gpu_id", true},
		{"gpu_open_gles_version", true},
		{"gpu_vendor", true},
		// Vulkan version is optional as older board may not supported it.
		{"gpu_vulkan_version", false},
		{"platform_cpu_vendor", true},
		{"platform_disk_size", true},
		{"platform_memory_size", true},
	} {
		_, ok := result.LabelsReporting[s.name]
		if ok {
			continue
		}
		if s.required {
			resultErr = errors.Wrapf(resultErr, "Required label %q is not found", s.name)
		} else {
			testing.ContextLogf(ctx, "Optional label %q is not found", s.name)
		}
	}
	return resultErr
}
