// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"strings"

	pb "go.chromium.org/tast-tests/cros/common/power/powerpb"
)

// FormatDeviceInfoForPowerLog create a map with specific keys used by power dashboard.
func FormatDeviceInfoForPowerLog(devInfo *pb.DeviceInfo) map[string]interface{} {
	board := devInfo.GetBoard()
	platform := devInfo.GetPlatform()

	if !strings.HasPrefix(platform, board) {
		board += "_" + platform
	}
	if devInfo.GetHasHammer() {
		board += "_hammer"
	}

	versionMap := map[string]interface{}{
		"hw":        devInfo.GetHardwareRevision(),
		"milestone": devInfo.GetChromeosReleaseMilestone(),
		"os":        devInfo.GetChromeosReleaseVersion(),
		"channel":   nil,
		"firmware":  devInfo.GetFirmwareVersion(),
		"ec":        devInfo.GetEcVersion(),
		"kernel":    devInfo.GetKernelVersion(),
		"arc":       devInfo.GetArcVersion(),
	}

	if devInfo.ChromeosChannel != nil {
		versionMap["channel"] = devInfo.GetChromeosChannel()
	}

	skuMap := map[string]interface{}{
		"cpu":                 devInfo.GetCpuName(),
		"memory_size":         devInfo.GetMemorySize(),
		"storage_size":        devInfo.GetDiskSize(),
		"display_resolution":  devInfo.GetScreenResolution(),
		"hwid":                devInfo.GetHardwareId(),
		"cpu_count":           devInfo.GetCpuCount(),
		"cpu_cores":           devInfo.GetCoreCount(),
		"cpu_threads":         devInfo.GetThreadCountPerCpu(),
		"cpu_vendor":          devInfo.GetCpuVendor(),
		"cpu_cache":           devInfo.GetCpuCacheSize(),
		"gpu":                 devInfo.GetGpuModel(),
		"memory_type":         devInfo.GetMemoryType(),
		"memory_frequency":    devInfo.GetMemoryFrequency(),
		"storage_type":        devInfo.GetStorageType(),
		"screen_size":         devInfo.GetScreenSize(),
		"screen_refresh_rate": devInfo.GetScreenRefreshRate(),
	}

	if devInfo.BatteryShutdownPercent != nil {
		skuMap["battery_shutdown_percent"] = devInfo.GetBatteryShutdownPercent()
	}

	if devInfo.BatterySize != nil {
		skuMap["battery_size"] = devInfo.GetBatterySize()
	}

	inaMap := map[string]interface{}{
		"version": 0,
		// TODO: Add 'ina' : power_rails.
	}

	// When you add keys to this schema, please also make sure the corresponding
	// unit test is covered in DeviceInfoUtilCheck().
	return map[string]interface{}{
		"board":   board,
		"version": versionMap,
		"sku":     skuMap,
		"ina":     inaMap,
		// note: note to annotate results on the dashboard.
		"note": devInfo.GetDashboardNote(),
	}
}
