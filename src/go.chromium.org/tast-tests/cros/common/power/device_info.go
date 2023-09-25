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
	board := devInfo.Board
	platform := devInfo.Platform

	if !strings.HasPrefix(platform, board) {
		board += "_" + platform
	}
	if devInfo.HasHammer {
		board += "_hammer"
	}

	versionMap := map[string]interface{}{
		"hw":        devInfo.HardwareRevision,
		"milestone": devInfo.ChromeosReleaseMilestone,
		"os":        devInfo.ChromeosReleaseVersion,
		"channel":   nil,
		"firmware":  devInfo.FirmwareVersion,
		"ec":        devInfo.EcVersion,
		"kernel":    devInfo.KernelVersion,
	}

	if devInfo.ChromeosChannel != nil {
		versionMap["channel"] = *devInfo.ChromeosChannel
	}

	skuMap := map[string]interface{}{
		"cpu":                      devInfo.CpuName,
		"memory_size":              devInfo.MemorySize,
		"storage_size":             devInfo.DiskSize,
		"display_resolution":       devInfo.ScreenResolution,
		"hwid":                     devInfo.HardwareId,
		"cpu_count":                devInfo.CpuCount,
		"cpu_cores":                devInfo.CoreCount,
		"cpu_threads":              devInfo.ThreadCountPerCpu,
		"cpu_vendor":               devInfo.CpuVendor,
		"cpu_cache":                devInfo.CpuCacheSize,
		"gpu":                      devInfo.GpuModel,
		"memory_type":              devInfo.MemoryType,
		"memory_frequency":         devInfo.MemoryFrequency,
		"storage_type":             devInfo.StorageType,
		"screen_size":              devInfo.ScreenSize,
		"screen_refresh_rate":      devInfo.ScreenRefreshRate,
		"battery_shutdown_percent": nil,
		"battery_size":             nil,
	}

	if devInfo.BatteryShutdownPercent != nil {
		skuMap["battery_shutdown_percent"] = *devInfo.BatteryShutdownPercent
	}

	if devInfo.BatterySize != nil {
		skuMap["battery_size"] = *devInfo.BatterySize
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
		"note": devInfo.DashboardNote,
	}
}
