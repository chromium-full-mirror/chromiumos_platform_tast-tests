// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"fmt"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/perf"
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
			ExtraAttr: []string{"group:cq-medium"},
		}, {
			Name:      "informational",
			Val:       true,
			ExtraAttr: []string{"informational"},
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

	pv := perf.NewValues()
	for i, device := range result.VGADevice {
		s.Logf("PCI vendor:device %v:%v", device.VendorID, device.DeviceID)
		vendorID, err := strconv.ParseInt(device.VendorID, 16, 64)
		if err != nil {
			s.Fatalf("Failed to convert vendorID %v to decimal", device.VendorID)
		}
		deviceID, err := strconv.ParseInt(device.DeviceID, 16, 64)
		if err != nil {
			s.Fatalf("Failed to convert deviceID %v to decimal", device.DeviceID)
		}
		pv.Set(perf.Metric{
			Name: fmt.Sprintf("pciid_%v", i),
		}, float64(vendorID<<4+deviceID))
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf data: ", err)
	}
}
