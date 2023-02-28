// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strings"

	"chromiumos/tast/local/power/util"
	"chromiumos/tast/testing"
)

// GetDeviceInfo returns a map that contains the information of the DUT.
func GetDeviceInfo(ctx context.Context) map[string]interface{} {
	board := util.GetBoard()
	platform := util.GetPlatform(ctx)
	if !strings.HasPrefix(platform, board) {
		board += "_" + platform
	}
	if util.HasHammer(ctx) {
		board += "_hammer"
	}

	deviceInfo := map[string]interface{}{
		"board": board,
		"version": map[string]interface{}{
			"hw":        util.GetHardwareRevision(ctx),
			"milestone": util.GetChromeOSReleaseMilestone(),
			"os":        util.GetChromeOSReleaseVersion(),
			"channel":   util.GetChromeOSChannel(),
			"firmware":  util.GetFirmwareVersion(ctx),
			"ec":        util.GetECVersion(ctx),
			"kernel":    util.GetKernelVersion(ctx),
		},
		"ina": map[string]interface{}{
			"version": 0,
			// TODO: Add 'ina' : power_rails.
		},
		// TODO: Add 'notes' for Power Dashboard.
	}

	skuMap := map[string]interface{}{
		"cpu":                 util.GetCPUName(ctx),
		"memory_size":         util.GetMemTotalGB(ctx),
		"storage_size":        util.GetDiskSizeGB(ctx, util.GetRootDevice(ctx)),
		"display_resolution":  util.GetScreenResolution(ctx),
		"hwid":                util.GetHardwareID(ctx),
		"cpu_count":           util.GetCPUNum(),
		"cpu_cores":           util.GetCPUCore(ctx),
		"cpu_threads":         util.GetCPUThreads(ctx),
		"cpu_vendor":          util.GetCPUVendor(ctx),
		"cpu_cache":           util.GetCPUCacheSize(ctx),
		"gpu":                 util.GetGPUModel(ctx),
		"memory_type":         util.GetMemoryType(ctx),
		"memory_frequency":    util.GetMemoryFrequency(ctx),
		"storage_type":        util.GetStorageType(ctx),
		"screen_size":         util.GetScreenSize(ctx),
		"screen_refresh_rate": util.GetScreenRefreshRate(ctx),
	}

	if batteryPath, err := SysfsBatteryPath(ctx); err == nil {
		shutdownPercent, err := LowBatteryShutdownPercent(ctx)
		if err != nil {
			testing.ContextLog(ctx, "Invalid battery_shutdown_percent: ", err)
		}
		skuMap["battery_shutdown_percent"] = shutdownPercent

		batterySize, err := ReadBatterySize(ctx, batteryPath)
		if err != nil {
			testing.ContextLog(ctx, "Invalid battery_size: ", err)
		}
		skuMap["battery_size"] = batterySize
	}
	deviceInfo["sku"] = skuMap
	return deviceInfo
}
