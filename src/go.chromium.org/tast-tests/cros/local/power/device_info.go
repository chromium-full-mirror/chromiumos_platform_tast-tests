// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"
	"strconv"

	cp "go.chromium.org/tast-tests/cros/common/power"
	pb "go.chromium.org/tast-tests/cros/common/power/powerpb"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/util"

	"go.chromium.org/tast/core/testing"
)

// GetDeviceInfo returns a struct that contains the information of the DUT.
func GetDeviceInfo(ctx context.Context, args ...OptionalRecorderArg) *pb.DeviceInfo {
	devInfo := new(pb.DeviceInfo)
	*devInfo = pb.DeviceInfo{
		Board:                    util.GetBoard(),
		Platform:                 util.GetPlatform(ctx),
		HasHammer:                util.HasHammer(ctx),
		HardwareRevision:         util.GetHardwareRevision(ctx),
		ChromeosReleaseMilestone: util.GetChromeOSReleaseMilestone(),
		ChromeosReleaseVersion:   util.GetChromeOSReleaseVersion(),
		FirmwareVersion:          util.GetFirmwareVersion(ctx),
		EcVersion:                util.GetECVersion(ctx),
		KernelVersion:            util.GetKernelVersion(ctx),
		DashboardNote:            getNote(args),
		CpuName:                  util.GetCPUName(ctx),
		MemorySize:               util.GetMemTotalGB(ctx),
		DiskSize:                 util.GetDiskSizeGB(ctx, util.GetRootDevice(ctx)),
		ScreenResolution:         util.GetScreenResolution(ctx),
		HardwareId:               util.GetHardwareID(ctx),
		CpuCount:                 util.GetCPUNum(),
		CoreCount:                util.GetCPUCore(ctx),
		ThreadCountPerCpu:        util.GetCPUThreads(ctx),
		CpuVendor:                util.GetCPUVendor(ctx),
		CpuCacheSize:             util.GetCPUCacheSize(ctx),
		GpuModel:                 util.GetGPUModel(ctx),
		MemoryType:               util.GetMemoryType(ctx),
		MemoryFrequency:          util.GetMemoryFrequency(ctx),
		StorageType:              util.GetStorageType(ctx),
		ScreenSize:               util.GetScreenSize(ctx),
		ScreenRefreshRate:        util.GetScreenRefreshRate(ctx),
		ArcVersion:               util.ArcVersion(),
	}

	if path, err := metrics.SysfsBatteryPath(ctx); err == nil {
		if val, err := metrics.LowBatteryShutdownPercent(ctx); err == nil {
			devInfo.BatteryShutdownPercent = new(float64)
			*devInfo.BatteryShutdownPercent = val
		} else {
			testing.ContextLog(ctx, "Invalid battery_shutdown_percent: ", err)
		}

		if val, err := metrics.ReadBatteryDesignEnergySize(ctx, path); err == nil {
			devInfo.BatterySize = new(float64)
			*devInfo.BatterySize = val
		} else {
			testing.ContextLog(ctx, "Invalid battery_size: ", err)
		}
	}

	if val := util.GetChromeOSChannel(); val != "" {
		devInfo.ChromeosChannel = new(string)
		*devInfo.ChromeosChannel = val
	}

	return devInfo
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

	deviceInfo := cp.FormatDeviceInfoForPowerLog(GetDeviceInfo(ctx))

	// board name should not be empty.
	const boardPattern = `\S+`
	re := regexp.MustCompile(boardPattern)
	if !re.MatchString(deviceInfo["board"].(string)) {
		failed = append(failed, "board: "+deviceInfo["board"].(string))
	}

	// read check for strings.
	for key, reStr := range stringCheckReg {
		if readResult, ok := deviceInfo["version"].(map[string]interface{})[key]; ok {
			if key == "channel" && readResult == nil {
				continue
			}
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
			// readResult as an interface{} could be int32, int64 or float64.
			if num, typeOk := readResult.(int32); typeOk && num <= 0 {
				failed = append(failed, key+": "+strconv.FormatInt(int64(num), 10))
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
		rate := deviceInfo["sku"].(map[string]interface{})["screen_refresh_rate"].(int32)
		if rate <= 0 {
			failed = append(failed, "screen_refresh_rate"+": "+strconv.FormatInt(int64(rate), 10))
		}
	}

	return failed
}

func getNote(args []OptionalRecorderArg) string {
	for _, recorderArg := range args {
		if recorderArg.argName == "pdash_note" {
			return recorderArg.argValue.(string)
		}
	}

	return ""
}
