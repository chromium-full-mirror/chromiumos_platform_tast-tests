// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/local/power/util"

	"go.chromium.org/tast/core/testing"
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

	// When you add keys to this schema, please also make sure the corresponding
	// unit test is covered in DeviceInfoUtilCheck().
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

		batterySize, err := ReadBatteryDesignEnergySize(ctx, batteryPath)
		if err != nil {
			testing.ContextLog(ctx, "Invalid battery_size: ", err)
		}
		skuMap["battery_size"] = batterySize
	}
	deviceInfo["sku"] = skuMap
	return deviceInfo
}

// DeviceInfoUtilCheck is a unit test for power/util. Return a list of util reads that
// failed the checks.
func DeviceInfoUtilCheck(ctx context.Context) []string {
	var failed = make([]string, 0)

	// regexp check for reads as strings.
	stringCheckReg := map[string]string{
		// version map
		"milestone": `\d+`,                        // milestone should be a string with only digits.
		"os":        `\S+`,                        // os should not be empty.
		"channel":   `(?i)beta|canary|dev|stable`, // channel names explicitly available.
		"firmware":  `(?i)Google_\S+.\d+.\d+.\d+`, // firmware name should be "Google_${name}.${os version}"".
		"ec":        `\S+`,                        // ec name should not be empty.
		"kernel":    `\d+.\d+.`,                   // kernel name should at least include version number "#.#".

		// sku map
		"cpu":          `\S+`,                        // cpu name should not be empty.
		"hwid":         `\S+`,                        // hwid should not be empty.
		"cpu_vendor":   `(?i)Intel|AMD|ARM|Qualcomm`, // cpu vendor should be one of major vendors.
		"gpu":          `\S+`,                        // gpu name should not be empty.
		"memory_type":  `\S+`,                        // memory type should not be empty.
		"storage_type": `\S+`,                        // storage type should not be empty.
	}
	// number check for reads as float/int.
	// TODO: b/282991186 - Add "memory_frequency" to numberCheck when reading
	// RAM frequency on ARM-based ChromeOS devices is available.
	numberCheck := []string{"cpu_count", "cpu_cores", "cpu_cores", "cpu_threads",
		"memory_size", "storage_size"}

	deviceInfo := GetDeviceInfo(ctx)

	// board name should not be empty.
	const boardPattern = `\S+`
	re := regexp.MustCompile(boardPattern)
	if !re.MatchString(deviceInfo["board"].(string)) {
		failed = append(failed, "board: "+deviceInfo["board"].(string))
	}

	// read check for strings.
	for key, reStr := range stringCheckReg {
		if readResult, ok := deviceInfo["version"].(map[string]interface{})[key]; ok {
			re := regexp.MustCompile(reStr)
			if !re.MatchString(readResult.(string)) {
				failed = append(failed, key+": "+readResult.(string))
			}
		}
		if readResult, ok := deviceInfo["sku"].(map[string]interface{})[key]; ok {
			re := regexp.MustCompile(reStr)
			if !re.MatchString(readResult.(string)) {
				failed = append(failed, key+": "+readResult.(string))
			}
		}
	}

	// read check for numbers.
	for _, key := range numberCheck {
		if readResult, ok := deviceInfo["sku"].(map[string]interface{})[key]; ok {
			// readResult as an interface{} could be int, int64 or float64.
			if num, typeOk := readResult.(int); typeOk && num <= 0 {
				failed = append(failed, key+": "+strconv.Itoa(num))
			}
			if num, typeOk := readResult.(int64); typeOk && num <= 0 {
				failed = append(failed, key+": "+strconv.FormatInt(num, 10))
			}
			if num, typeOk := readResult.(float64); typeOk && num <= 0 {
				failed = append(failed, key+": "+strconv.FormatFloat(num, 'g', -1, 64))
			}
		}
	}

	// Additioanl checks for device that has a screen.
	if util.HasScreen(ctx) {
		const screenReStr = `\d+x\d+` // display resolution and screen size should be #x#.
		re := regexp.MustCompile(screenReStr)
		readResult := deviceInfo["sku"].(map[string]interface{})["display_resolution"].(string)
		if !re.MatchString(readResult) {
			failed = append(failed, "display_resolution"+": "+readResult)
		}
		readResult = deviceInfo["sku"].(map[string]interface{})["screen_size"].(string)
		if !re.MatchString(readResult) {
			failed = append(failed, "screen_size"+": "+readResult)
		}
		rate := deviceInfo["sku"].(map[string]interface{})["screen_refresh_rate"].(int)
		if rate <= 0 {
			failed = append(failed, "screen_refresh_rate"+": "+strconv.Itoa(rate))
		}
	}

	return failed
}
