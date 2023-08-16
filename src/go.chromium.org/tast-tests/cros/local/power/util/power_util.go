// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util contains the helper utils functions.
package util

import (
	"context"
	"math"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/graphics/hardwareprobe"
	"go.chromium.org/tast/core/testing"
)

// GetBoardProperty gets a specific property from /etc/lsb-release.
func GetBoardProperty(key string) string {
	f, err := os.ReadFile("/etc/lsb-release")
	if err != nil {
		return ""
	}
	pattern := `(?m)^` + key + `=(.*)`
	re := regexp.MustCompile(pattern)
	submatchGroup := re.FindStringSubmatch(string(f))
	// A legitimate submatchGroup would be ["key=result", "result"]
	if len(submatchGroup) < 2 {
		return ""
	}
	return strings.TrimSpace(submatchGroup[1])
}

// GetBoard returns the release board name.
func GetBoard() string {
	return GetBoardProperty("CHROMEOS_RELEASE_BOARD")
}

// GetPlatform returns the ChromeOS platform name.
// For unibuild this should be model name.
// For non-unibuild it will either be board name or empty string.
func GetPlatform(ctx context.Context) string {
	readResult, err := testexec.CommandContext(ctx, "cros_config", "/", "name").Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get platform name")
		return ""
	}
	platform := string(readResult)
	if platform == "" {
		return GetBoard()
	}
	return platform
}

// GetChromeOSReleaseMilestone returns the chromeos milestone.
func GetChromeOSReleaseMilestone() string {
	return GetBoardProperty("CHROMEOS_RELEASE_CHROME_MILESTONE")
}

// GetChromeOSReleaseVersion returns the chromeos version.
func GetChromeOSReleaseVersion() string {
	return GetBoardProperty("CHROMEOS_RELEASE_VERSION")
}

// GetFirmwareVersion returns the firmware version.
func GetFirmwareVersion(ctx context.Context) string {
	readResult, err := testexec.CommandContext(ctx, "crossystem", "fwid").Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get firmware version")
		return ""
	}
	return strings.TrimSpace(string(readResult))
}

// GetECVersion returns the EC version.
func GetECVersion(ctx context.Context) string {
	const path = "/dev/cros_ec"
	f, err := os.ReadFile(path)
	if err != nil {
		testing.ContextLog(ctx, "Device does not have a chromium EC")
		return getWilcoECVersion(ctx)
	}
	ecInfo := strings.Split(strings.TrimSpace(string(f)), "\n")
	if len(ecInfo) != 4 {
		testing.ContextLogf(ctx, "Failed to parse EC dev file: %s has %d lines", path, len(ecInfo))
		return ""
	}
	activeCopy := ecInfo[3]
	if activeCopy == "read-only" {
		return ecInfo[1]
	} else if activeCopy == "read-write" {
		return ecInfo[2]
	} else {
		testing.ContextLogf(ctx, "Failed to determine active EC copy: %s", activeCopy)
		return ""
	}
}

// getWilcoECVersion reads the version of Wilco EC on Drallion and Sarien board.
// See details in go/wilco-ec.
func getWilcoECVersion(ctx context.Context) string {
	const cmd = "cbmem -c | grep -m 1 EC.Label"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get Wilco EC version")
		return ""
	}
	ecInfo := strings.Split(strings.TrimSpace(string(readResult)), ":")
	if len(ecInfo) != 2 {
		testing.ContextLogf(ctx, "Failed to parse Wilco EC version: %s", string(readResult))
		return ""
	}
	// To make format close to chromium EC.
	return "wilco_" + strings.TrimSpace(ecInfo[1])
}

// GetKernelVersion returns the kernel version.
func GetKernelVersion(ctx context.Context) string {
	readResult, err := testexec.CommandContext(ctx, "uname", "-r").Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get kernel version")
		return ""
	}
	return strings.TrimSpace(string(readResult))
}

// GetScreenResolution returns the screen resolution.
func GetScreenResolution(ctx context.Context) string {
	cmd := "for f in /sys/class/drm/*/*/modes; do head -1 $f; done"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get screen resolution, or device doesn't have a screen")
		return ""
	}
	return strings.TrimSpace(string(readResult))
}

// GetRootDevice returns the root disk device.
func GetRootDevice(ctx context.Context) string {
	readResult, err := testexec.CommandContext(ctx, "rootdev", "-s", "-d").Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get root disk device")
		return ""
	}
	return strings.TrimSpace(string(readResult))
}

const diskInfoPath = "/proc/partitions"

// GetDiskSizeKB returns the size of the given disk in kByte, and in float64.
func GetDiskSizeKB(ctx context.Context, diskName string) float64 {
	if diskName == "" {
		return 0
	}
	disk := path.Base(diskName)
	f, err := os.ReadFile(diskInfoPath)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to read partition %s file", diskName)
		return 0
	}
	pattern := `\s([0-9]+)\s` + disk
	re := regexp.MustCompile(pattern)
	submatchGroup := re.FindStringSubmatch(string(f))
	// A legitimate submatchGroup would be [" size disk", "size"]
	if len(submatchGroup) < 2 {
		return 0
	}
	diskSizeInFloat, err := strconv.ParseFloat(strings.TrimSpace(submatchGroup[1]), 64)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to parse partition %s read info to float", diskName)
		return 0
	}
	return diskSizeInFloat
}

// GetDiskSizeMB returns the size of the given disk in MByte, and in int64.
func GetDiskSizeMB(ctx context.Context, diskName string) int64 {
	return int64(math.Round(GetDiskSizeKB(ctx, diskName) / 1024))
}

// GetDiskSizeGB returns the size of the given disk in GByte, and in int64.
func GetDiskSizeGB(ctx context.Context, diskName string) int64 {
	return int64(math.Round(GetDiskSizeKB(ctx, diskName) / 1024 / 1024))
}

// GetHardwareID returns the hardware ID.
func GetHardwareID(ctx context.Context) string {
	readResult, err := testexec.CommandContext(ctx, "crossystem", "hwid").Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get hardware ID")
		return ""
	}
	return strings.TrimSpace(string(readResult))
}

// GetHardwareRevision returns the hardware revision.
func GetHardwareRevision(ctx context.Context) string {
	readResult, err := testexec.CommandContext(ctx, "crossystem", "board_id").Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get hardware revision from crossystem")
		// "crossystem board_id" only works on M110 or later.
		readResult, err := testexec.CommandContext(ctx, "mosys", "platform", "version").Output()
		if err != nil {
			testing.ContextLog(ctx, "Failed to get hardware revision from mosys")
			return ""
		}
		return strings.TrimSpace(string(readResult))
	}
	return "rev" + strings.TrimSpace(string(readResult))
}

// GetChromeOSChannel returns chromeos channel.
func GetChromeOSChannel() string {
	fullResult := GetBoardProperty("CHROMEOS_RELEASE_DESCRIPTION")
	pattern := `.+ (.+)-channel.*$`
	re := regexp.MustCompile(pattern)
	submatchGroup := re.FindStringSubmatch(fullResult)
	if len(submatchGroup) < 2 {
		return ""
	}
	return submatchGroup[1]
}

// HasHammer checks whether DUT has hammer device.
func HasHammer(ctx context.Context) bool {
	path := "/sys/bus/usb/devices/*/product"
	readResult, err := testexec.CommandContext(ctx, "grep", "Hammer", path).Output()
	if err != nil {
		testing.ContextLog(ctx, "Cannot grep hammer information")
		return false
	}
	if string(readResult) == "" {
		return false
	}
	return true
}

const memInfoPath = "/proc/meminfo"

// Memory metric identifier to use with GetMemInfoMetrics().
const (
	MemInfoMemTotal          = `MemTotal`
	MemInfoMemFree           = `MemFree`
	MemInfoMemAvailable      = `MemAvailable`
	MemInfoBuffers           = `Buffers`
	MemInfoCached            = `Cached`
	MemInfoSwapCached        = `SwapCached`
	MemInfoActive            = `Active`
	MemInfoInactive          = `Inactive`
	MemInfoHighTotal         = `HighTotal`
	MemInfoHighFree          = `HighFree`
	MemInfoLowTotal          = `LowTotal`
	MemInfoLowFree           = `LowFree`
	MemInfoSwapTotal         = `SwapTotal`
	MemInfoSwapFree          = `SwapFree`
	MemInfoDirty             = `Dirty`
	MemInfoWriteback         = `Writeback`
	MemInfoAnonPages         = `AnonPages`
	MemInfoMapped            = `Mapped`
	MemInfoShmem             = `Shmem`
	MemInfoKReclaimable      = `KReclaimable`
	MemInfoSlab              = `Slab`
	MemInfoSReclaimable      = `SReclaimable`
	MemInfoSUnreclaim        = `SUnreclaim`
	MemInfoPageTables        = `PageTables`
	MemInfoNFSUnstable       = `NFS_Unstable`
	MemInfoBounce            = `Bounce`
	MemInfoWritebackTmp      = `WritebackTmp`
	MemInfoCommitLimit       = `CommitLimit`
	MemInfoCommittedAS       = `Committed_AS`
	MemInfoVmallocTotal      = `VmallocTotal`
	MemInfoVmallocUsed       = `VmallocUsed`
	MemInfoVmallocChunk      = `VmallocChunk`
	MemInfoPercpu            = `Percpu`
	MemInfoHardwareCorrupted = `HardwareCorrupted`
	MemInfoAnonHugePages     = `AnonHugePages`
	MemInfoShmemHugePages    = `ShmemHugePages`
	MemInfoShmemPmdMapped    = `ShmemPmdMapped`
)

// GetMemInfoMetrics returns a map of integer metrics read from /proc/meminfo.
// The returned map contains only metrics that could be found and parsed.
// On error nil is returned.
func GetMemInfoMetrics(ctx context.Context, metrics []string) map[string]int64 {
	f, err := os.ReadFile(memInfoPath)
	if err != nil {
		testing.ContextLog(ctx, "Failed to read memory info file")
		return nil
	}
	// Match from an ASCII word boundary to prevent partial matches such
	// as PageTables -> SecPageTables.
	pattern := `\b(` + strings.Join(metrics, `|`) + `):\s*([0-9]+).*`
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(string(f), len(metrics))
	if matches == nil {
		testing.ContextLogf(ctx, "Failed to find metrics %s in memory info file", strings.Join(metrics, `, `))
		return nil
	}
	metricMap := map[string]int64{}

	for _, match := range matches {
		metric := match[1]
		value, err := strconv.ParseInt(match[2], 10, 64)
		if err != nil {
			testing.ContextLogf(ctx, "Failed to parse %s from memory info file", metric)
			continue
		}
		metricMap[metric] = value
	}
	return metricMap
}

// GetMemTotalKB returns the float total memory available in kBytes.
func GetMemTotalKB(ctx context.Context) float64 {
	value := GetMemInfoMetrics(ctx, []string{MemInfoMemTotal})
	// On error value is nil, len(nil) evaluates to 0
	if len(value) < 1 {
		testing.ContextLog(ctx, "Failed to find total memory in memory info file")
		return 0
	}
	return float64(value[MemInfoMemTotal])
}

// GetMemTotalMB returns the int total memory available in MBytes.
func GetMemTotalMB(ctx context.Context) int64 {
	return int64(math.Round(GetMemTotalKB(ctx) / 1024))
}

// GetMemTotalGB returns the int total memory available in GBytes.
func GetMemTotalGB(ctx context.Context) int64 {
	return int64(math.Round(GetMemTotalKB(ctx) / 1024 / 1024))
}

// GetCPUName returns the cpu name.
func GetCPUName(ctx context.Context) string {
	// Try get the cpu name from device tree first.
	if _, err := os.Stat("/proc/device-tree/compatible"); err == nil {
		cmd := "sed -e 's/\\x0/\\n/g' /proc/device-tree/compatible | tail -1"
		readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
		if err == nil {
			return strings.ReplaceAll(strings.TrimSpace(string(readResult)), ",", " ")
		}
	}

	// If path above does not work, next try get from "uname -p"
	readResult, err := testexec.CommandContext(ctx, "uname", "-p").Output()
	// 'uname -p' return variant of unknown or amd64 or x86_64 or i686.
	// grep the name from /proc/cpuinfo instead.
	checkPattern := `unknown|amd64|[ix][0-9]?86(_64)?`
	checkRE := regexp.MustCompile(checkPattern)
	if err != nil || checkRE.MatchString(string(readResult)) {
		grepCmd := "grep model.name /proc/cpuinfo | cut -f 2 -d: | head -1"
		readResult, err = testexec.CommandContext(ctx, "bash", "-c", grepCmd).Output()
		if err != nil {
			testing.ContextLog(ctx, "Could not find the CPU name")
			return ""
		}
	}
	// Remove bloat from CPU name, e.g.
	// Intel(R) Core(TM) i5-7Y57 CPU@1.2GHz          -> Intel Core i5-7Y57
	// AMD A10-7850K APU with Radeon(TM) R7 Graphics -> AMD A10-7850K
	trimPattern := `(?i) (@|processor|apu|soc|radeon).*|\(.*?\)| cpu`
	trimRE := regexp.MustCompile(trimPattern)
	trimmedResult := trimRE.ReplaceAll(readResult, []byte(""))
	return strings.TrimSpace(string(trimmedResult))
}

// GetCPUNum returns the number of CPUs.
func GetCPUNum() int {
	return runtime.NumCPU()
}

// GetCPUCore returns the number of cores per CPU according to lscpu.
// A returning of 0 means invalid.
func GetCPUCore(ctx context.Context) int {
	const cmd1 = "lscpu | grep 'Core(s)' | cut -d ':' -f 2"
	const cmd2 = "lscpu | grep 'Socket(s)' | cut -d ':' -f 2"
	readResult1, err1 := testexec.CommandContext(ctx, "bash", "-c", cmd1).Output()
	readResult2, err2 := testexec.CommandContext(ctx, "bash", "-c", cmd2).Output()
	if err1 != nil || err2 != nil {
		testing.ContextLog(ctx, "Failed to read lscpu")
		// return an invalid number
		return 0
	}
	corePerSocketList := strings.Split(strings.TrimSpace(string(readResult1)), "\n")
	socketList := strings.Split(strings.TrimSpace(string(readResult2)), "\n")
	var sum int64
	for i, core := range corePerSocketList {
		var singleCoresPerSocket, singleSocket int64
		var err error
		if singleCoresPerSocket, err = strconv.ParseInt(strings.TrimSpace(core), 10, 64); err != nil {
			testing.ContextLog(ctx, "Failed to get number of cores per socket: ", err)
		}
		if singleSocket, err = strconv.ParseInt(strings.TrimSpace(socketList[i]), 10, 64); err != nil {
			testing.ContextLog(ctx, "Failed to get number of socket: ", err)
		}
		sum += singleCoresPerSocket * singleSocket
	}
	return int(sum)
}

// GetCPUThreads return the threads per CPU.
// Minimum return is 1.
func GetCPUThreads(ctx context.Context) int {
	path := "/sys/devices/system/cpu/present"
	readResult, err := testexec.CommandContext(ctx, "cat", path).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get threads per CPU")
		return 1
	}
	// example readResut: []byte "0-3"
	fields := strings.Split(strings.TrimSpace(string(readResult)), "-")
	if len(fields) < 2 {
		testing.ContextLog(ctx, "Failed to parse thread numbers")
		return 1
	}
	num, err := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
	if err != nil {
		testing.ContextLog(ctx, "Failed to parse thread numbers")
		return 1
	}
	return int(num + 1)
}

// GetCPUVendor returns the vendor of CPU.
func GetCPUVendor(ctx context.Context) string {
	cmd := "lscpu | grep 'Vendor' | cut -d ':' -f 2"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get CPU vendor")
		return ""
	}
	return strings.TrimSpace(string(readResult))
}

// GetCPUCacheSize returns CPU L3 cache size in MByte.
func GetCPUCacheSize(ctx context.Context) float64 {
	path := "/sys/devices/system/cpu/cpu0/cache/index3/size"
	readResult, err := testexec.CommandContext(ctx, "cat", path).Output()
	if err != nil {
		testing.ContextLog(ctx, "Device does not have a L3 cache")
		return 0
	}
	if strings.Contains(string(readResult), "K") {
		num := strings.Split(string(readResult), "K")[0]
		numFloat, err := strconv.ParseFloat(num, 64)
		if err != nil {
			testing.ContextLog(ctx, "Failed to parse CPU cache result")
			return 0
		}
		return math.Round(numFloat / 1024)
	} else if strings.Contains(string(readResult), "M") {
		num := strings.Split(string(readResult), "M")[0]
		numFloat, err := strconv.ParseFloat(num, 64)
		if err != nil {
			testing.ContextLog(ctx, "Failed to parse CPU cache result")
			return 0
		}
		return math.Round(numFloat)
	} else {
		return 0
	}
}

// GetGPUModel returns the GPU model name.
func GetGPUModel(ctx context.Context) string {
	cmd := "lshw -businfo | grep -i display"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get GPU model from lshw")
		return getGPUModelOnARM(ctx)
	}
	fields := strings.Split(string(readResult), "display")
	if len(fields) < 2 {
		testing.ContextLog(ctx, "Failed to parse GPU model name from lshw")
		return getGPUModelOnARM(ctx)
	}
	return strings.TrimSpace(fields[1])
}

// getGPUModelOnARM reads the GPU vendor + GPU family on ARM.
// Currently ARM-based ChromeOS devices does not have display/graphics sections
// in lshw so we need a workaround to read from system.
func getGPUModelOnARM(ctx context.Context) string {
	result, err := hardwareprobe.GetHardwareProbeResult(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get HardwareProbe result")
		return ""
	}
	return result.GPUInfo[0].Vendor + "_" + result.GPUInfo[0].Family
}

// GetMemoryType returns the memory type e.g. LPDDR3
func GetMemoryType(ctx context.Context) string {
	cmd := "mosys -s dram memory spd print type | uniq"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get memory type")
		return ""
	}
	return strings.TrimSpace(string(readResult))
}

// GetMemoryFrequency returns the memory frequency in MT/s.
func GetMemoryFrequency(ctx context.Context) int {
	cmd := "dmidecode --type memory | grep $'\tSpeed:' | uniq | cut -d ':' -f 2"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil || !strings.Contains(string(readResult), "MT") {
		testing.ContextLog(ctx, "Failed to get memory frequency")
		return 0
	}
	fields := strings.Split(string(readResult), "MT")
	num, err := strconv.ParseInt(strings.TrimSpace(fields[0]), 10, 64)
	if err != nil {
		testing.ContextLog(ctx, "Failed to parse memory frequency")
		return 0
	}
	return int(num)
}

// GetStorageType returns the storage type according to rootdev.
func GetStorageType(ctx context.Context) string {
	readResult, err := testexec.CommandContext(ctx, "rootdev", "-s").Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get storage type")
		return ""
	}
	return strings.TrimSpace(path.Base(string(readResult)))
}

// HasScreen detects if the device has a screen.
func HasScreen(ctx context.Context) bool {
	cmd := "for f in /sys/class/drm/*/*/modes; do head -1 $f; done"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil || strings.TrimSpace(string(readResult)) == "" {
		return false
	}
	return true
}

// GetScreenSize returns the screen size.
func GetScreenSize(ctx context.Context) string {
	if !HasScreen(ctx) {
		return ""
	}
	cmd := "modetest -c | sed -n '/size (mm)/{n;p}'"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get screen size")
		return ""
	}
	pattern := "[0-9]+x[0-9]+"
	re := regexp.MustCompile(pattern)
	findResult := re.Find(readResult)
	return string(findResult)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetScreenRefreshRate returns the screen refresh rate in Hz.
func GetScreenRefreshRate(ctx context.Context) int {
	if !HasScreen(ctx) {
		return 0
	}
	cmd := "modetest -c | grep 'refresh (Hz)'"
	readResult, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get screen refresh rate")
		return 0
	}
	pattern := `\s+`
	re := regexp.MustCompile(pattern)
	re.ReplaceAll(readResult, []byte(" "))
	fields1 := strings.Split(strings.TrimSpace(string(readResult)), " ")

	cmd = "modetest -c | sed -n '/refresh (Hz)/{n;p}'"
	readResult, err = testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get screen refresh rate")
		return 0
	}
	re.ReplaceAll(readResult, []byte(" "))
	fields2 := strings.Split(strings.TrimSpace(string(readResult)), " ")
	// e.g. fields1 = ["index", "name", "refresh", ...]
	// e.g. fields2 = ["#0", "1920x1080", "60.00", ...]
	for i := 0; i < min(len(fields1), len(fields2)); i++ {
		if fields1[i] == "refresh" {
			findResult, err := strconv.ParseFloat(fields2[i], 64)
			if err != nil {
				testing.ContextLog(ctx, "Failed to get screen refresh rate")
				return 0
			}
			return int(math.Round(findResult))
		}
	}
	testing.ContextLog(ctx, "Failed to get screen refresh rate")
	return 0
}

// GetBacklightLevel returns screen backlight brightness in non-linear (human
// perception) and linear percent respectively.
func GetBacklightLevel(ctx context.Context) (nonlinear, linear float64) {
	const (
		backlight     = "backlight_tool"
		brightnessArg = "--get_brightness"
		nonlinearArg  = "--level_to_nonlinear"
		linearArg     = "--level_to_linear"
	)

	rawLevel, err := testexec.CommandContext(ctx, backlight, brightnessArg).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get backlight brightness")
		return nonlinear, linear
	}

	readResult, err := testexec.CommandContext(ctx, backlight, nonlinearArg+"="+strings.TrimSpace(string(rawLevel))).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get non-linear backlight percentage")
	}
	nonlinear, err = strconv.ParseFloat(strings.TrimSpace(string(readResult)), 64)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to parse backlight read %s to float", readResult)
	}

	readResult, err = testexec.CommandContext(ctx, backlight, linearArg+"="+strings.TrimSpace(string(rawLevel))).Output()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get linear backlight percentage")
	}
	linear, err = strconv.ParseFloat(strings.TrimSpace(string(readResult)), 64)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to parse backlight read %s to float", readResult)
	}

	return nonlinear, linear
}

// TODO: b/281632991 - Consider remove this list once chargeoverride is verified
// to be working on these boards.
var boardsCannotChargeOverride = []string{"jacuzzi", "kukui"}

// SupportChargeOverride returns a boolean indicating if 'ectool chargeoverride'
// is supported on the board.
func SupportChargeOverride() bool {
	board := GetBoard()
	for _, boardPrefix := range boardsCannotChargeOverride {
		if strings.HasPrefix(board, boardPrefix) {
			return false
		}
	}
	return true
}
