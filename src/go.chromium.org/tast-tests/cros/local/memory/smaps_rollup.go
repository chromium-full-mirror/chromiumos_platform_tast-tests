// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// NameToCategoryMetricsMap maps a process category name to its key PSS metrics.
type NameToCategoryMetricsMap map[string]*CategoryHostMetrics

// CategoryHostMetrics has a summary of host HostMetrics
// we keep on a per-category basis for reporting.
// All values in Kilobytes.
type CategoryHostMetrics struct {
	Pss      uint64
	PssSwap  uint64
	PssGuest uint64
}

// HostSummary captures a few key data items that are used to compute
// overall system memory status.
// All values are expressed in Kilobytes.
type HostSummary struct {
	MemTotal        uint64
	MemFree         uint64
	HostCached      uint64
	LinuxUsed       uint64
	CategoryMetrics NameToCategoryMetricsMap
}

// SharedInfo holds shared memory use information for one process.
// SharedSwapPss is the amount of swap used by shared memory regions divided by
// the number of times those regions are mapped.
// CrosvmGuestPss is the sum of the Pss used by the crosvm_guest region,
//
//	(which means memory from the VM mapped on the host).
type SharedInfo struct {
	SharedSwapPss  uint64
	CrosvmGuestPss uint64
}

// CrosVMSmapsRollup stores smaps_rollup for each process belonging to a
// specific crosvm main process. The VM can be identified by tag (ARCVM, VM).
type CrosVMSmapsRollup struct {
	CrosVMPid int32
	Tag       string
	Rollups   []*NamedSmapsRollup
}

// FullSmapsRollup holds smaps_rollup info for both host and ARCVM guest
// processes. Host processes for crosvm are stored separately in CrosVMRollups.
type FullSmapsRollup struct {
	HostRollups   []*NamedSmapsRollup
	CrosVMRollups []*CrosVMSmapsRollup
	ARCVMRollups  []*NamedSmapsRollup
}

// NamedSmapsRollup is a SmapsRollup plus the process name and ID, and
// information on shared memory use (Shared field).
type NamedSmapsRollup struct {
	Command string
	Pid     int32
	Shared  *SharedInfo
	Rollup  map[string]uint64
}

// SharedInfoMap maps process ids to shared memory information.
type SharedInfoMap map[int32]*SharedInfo

var smapsRollupRE = regexp.MustCompile(`(?m)^([^:]+):\s*(\d+)\s*kB$`)

// NewSmapsRollup parses the contents of a /proc/<pid>/smaps_rollup file. All
// sizes are in kilobytes.
func NewSmapsRollup(smapsRollupFileData []byte) (map[string]uint64, error) {
	result := make(map[string]uint64)
	matches := smapsRollupRE.FindAllSubmatch(smapsRollupFileData, -1)
	if matches == nil {
		return nil, errors.Errorf("failed to parse smaps_rollup file %q", string(smapsRollupFileData))
	}
	for _, match := range matches {
		field := string(match[1])
		kbString := string(match[2])
		kb, err := strconv.ParseUint(kbString, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse %q value from smaps_rollup: %q", field, kbString)
		}
		result[field] = kb
	}
	return result, nil
}

// smapsRollups returns a NamedSmapsRollup for every process in processes.
// It also fills the passed summary struct with PSS data about the crosvm * processes.
// Sizes are in kilobytes.
// The Shared field is initialized from sharedInfoMap, if provided.
func smapsRollups(ctx context.Context, processes []*process.Process, sharedInfoMap SharedInfoMap, summary *HostSummary) ([]*NamedSmapsRollup, error) {
	rollups := make([]*NamedSmapsRollup, len(processes))
	g, ctx := errgroup.WithContext(ctx)
	for index, process := range processes {
		// All these are captured by value in the closure - and that is what we want.
		i := index
		p := process

		g.Go(func() error {
			// We're racing with this process potentially exiting, so just
			// ignore errors and don't generate a NamesSmapsRollup if we fail to
			// read anything from proc.
			smapsData, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/smaps_rollup", p.Pid))
			if err != nil {
				// Not all processes have a smaps_rollup, this process may have
				// exited.
				return nil
			} else if len(smapsData) == 0 {
				// On some processes, smaps_rollups exists but is empty.
				return nil
			}
			command, err := p.Cmdline()
			if err != nil {
				// Process may have died between reading smapsData and now, so
				// just ignore errors here.
				testing.ContextLogf(ctx, "SmapsRollups failed to get Cmdline for process %d: %s", p.Pid, err)
				return nil
			}
			rollup, err := NewSmapsRollup(smapsData)
			if err != nil {
				return errors.Wrapf(err, "failed to parse /proc/%d/smaps_rollup", p.Pid)
			}
			rollups[i] = &NamedSmapsRollup{
				Command: command,
				Pid:     p.Pid,
				Rollup:  rollup,
				Shared:  sharedInfoMap[p.Pid],
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, errors.Wrap(err, "failed to wait for all smaps_rollup parsing to be done")
	}
	var result []*NamedSmapsRollup
	for _, rollup := range rollups {
		if rollup != nil {
			result = append(result, rollup)
		}
	}
	return result, nil
}

// sharedSwapPssRE matches smaps entries that are mapped shared, with the
// following match groups:
// [1] The name of the mapping.
// [2] The RSS for that mapping within this process, in kIB
// [3] The PSS for that mapping within this process, in kIB
// [4] The size of swapped out pages in the mapping, in kiB.
var sharedSwapPssRE = regexp.MustCompile(`(?m)^[[:xdigit:]]+-[[:xdigit:]]+ [-r][-w][-x]s [[:xdigit:]]+ [[:xdigit:]]+:[[:xdigit:]]+ ([\d]+) +(\S[^\n]*)$
(?:^\w+: +[^\n]+$
)*^Rss: +(\d+) kB$
(?:^\w+: +[^\n]+$
)*^Pss: +(\d+) kB$
(?:^\w+: +[^\n]+$
)*^Swap: +(\d+) kB$`)

// SharedMapping contains information parsed from a smaps entry. Numbers are KiB.
type SharedMapping struct {
	name  string
	swap  uint64
	pss   uint64
	rss   uint64
	inode uint64
}

// ParseSmapsData parses the given smaps data.
func ParseSmapsData(data []byte) ([]SharedMapping, error) {
	var swaps []SharedMapping
	matches := sharedSwapPssRE.FindAllSubmatch(data, -1)
	for _, match := range matches {
		inode, err := strconv.ParseUint(string(match[1]), 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse inode %q", match[1])
		}
		name := string(match[2])
		rssKiB, err := strconv.ParseUint(string(match[3]), 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse rss value %q", match[3])
		}
		pssKiB, err := strconv.ParseUint(string(match[4]), 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse pss value %q", match[4])
		}
		swapKiB, err := strconv.ParseUint(string(match[5]), 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse swap value %q", match[5])
		}
		rss := rssKiB
		pss := pssKiB
		swap := swapKiB
		swaps = append(swaps, SharedMapping{name, swap, pss, rss, inode})
	}
	return swaps, nil
}

// makeSharedInfoMap creates a map from Pid to the amount of SwapPss used by shared
// mappings per process. The SwapPss field in smaps_rollup does not include
// memory swapped out of shared mappings. In order to calculate a complete
// SwapPss, we parse smaps for all shared mappings in all processes, and then
// divide their "Swap" value by the number of times the shared memory is mapped.
func makeSharedInfoMap(ctx context.Context, processes []*process.Process) (SharedInfoMap, error) {
	g, ctx := errgroup.WithContext(ctx)
	procSwaps := make([][]SharedMapping, len(processes))
	for index, process := range processes {
		i := index
		pid := process.Pid
		g.Go(func() error {
			smapsData, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/smaps", pid))
			if err != nil {
				// Not all processes have a smaps_rollup, this process may have
				// exited.
				return nil
			}
			smaps, err := ParseSmapsData(smapsData)
			if err != nil {
				return err
			}
			procSwaps[i] = smaps
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, errors.Wrap(err, "failed to wait for all smaps parsing to be done")
	}
	// Count how many times each shared mapping has been mapped.
	mapCount := make(map[string]uint64)
	for _, swaps := range procSwaps {
		for _, swap := range swaps {
			mapCount[swap.name]++
		}
	}
	// Use the counts to divide each mapping's swap size to compute SwapPss.
	// Also stack up each process's share of the crosvm_guest mapping
	sharedInfoMap := make(SharedInfoMap)
	for i, swaps := range procSwaps {
		pid := processes[i].Pid
		sharedInfo := &SharedInfo{}
		sharedInfoMap[pid] = sharedInfo
		for _, swap := range swaps {
			if strings.HasPrefix(swap.name, "/memfd:crosvm_guest") {
				sharedInfo.CrosvmGuestPss += swap.pss
			}
			sharedInfo.SharedSwapPss += swap.swap / mapCount[swap.name]
		}
	}
	return sharedInfoMap, nil
}

type processCategory struct {
	commandRE *regexp.Regexp
	name      string
}

// processCategories defines categories used to aggregate per-process memory
// metrics. The first commandRE to match a process' command line defines its
// category.
var processCategories = []processCategory{
	{
		commandRE: regexp.MustCompile(`^/usr/bin/crosvm run.*/arcvm.sock`),
		name:      "crosvm_arcvm",
	}, {
		commandRE: regexp.MustCompile(`^/usr/bin/crosvm`),
		name:      "crosvm_other",
	}, {
		commandRE: regexp.MustCompile(`^/opt/google/chrome/chrome.*--type=renderer`),
		name:      "chrome_renderer",
	}, {
		commandRE: regexp.MustCompile(`^/opt/google/chrome/chrome.*--type=gpu-process`),
		name:      "chrome_gpu",
	}, {
		commandRE: regexp.MustCompile(`^/opt/google/chrome/chrome.*--type=`),
		name:      "chrome_other",
	}, { // The Chrome browser is the only chrome without a --type argument.
		commandRE: regexp.MustCompile(`^/opt/google/chrome/chrome`),
		name:      "chrome_browser",
	}, {
		commandRE: regexp.MustCompile(`.*`),
		name:      "other",
	},
}

// GetHostMetrics parses smaps and smaps_rollup information from every
// running process on the ChromeOS (host) side.
// Values are summarized according to the processCategories defined above and
// returned in the HostSummary structure.
// If outdir is provided, detailed rollup information is also written
// to files in that directory.
func GetHostMetrics(ctx context.Context, outdir, suffix string) (*HostSummary, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get all processes")
	}

	sharedInfoMap, err := makeSharedInfoMap(ctx, processes)
	if err != nil {
		return nil, err
	}

	summary := &HostSummary{CategoryMetrics: make(NameToCategoryMetricsMap)}

	rollups, err := smapsRollups(ctx, processes, sharedInfoMap, summary)
	if err != nil {
		return nil, err
	}

	meminfo, err := kernelmeter.ReadMemInfo()
	if err != nil {
		return nil, err
	}

	if len(outdir) > 0 {
		// Dump intermediate data.
		rollupsJSON, err := json.MarshalIndent(rollups, "", "  ")
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert smaps_rollups to JSON")
		}
		filename := fmt.Sprintf("smaps_rollup%s.json", suffix)
		if err := ioutil.WriteFile(path.Join(outdir, filename), rollupsJSON, 0644); err != nil {
			return nil, errors.Wrapf(err, "failed to write smaps_rollups to %s", filename)
		}
	}

	// Convert reported totals in bytes into the common KiB unit.
	summary.MemTotal = uint64(meminfo["MemTotal"]) / KiB
	summary.MemFree = uint64(meminfo["MemFree"]) / KiB
	summary.HostCached = uint64(meminfo["SReclaimable"]+meminfo["Buffers"]+meminfo["Cached"]-meminfo["Mapped"]) / KiB
	summary.LinuxUsed = summary.MemTotal - uint64(meminfo["MemAvailable"])/KiB

	metrics := summary.CategoryMetrics // Shallow copy, as it is a map.
	for _, rollup := range rollups {
		for _, category := range processCategories {
			if category.commandRE.MatchString(rollup.Command) {
				metric := metrics[category.name]
				if metric == nil {
					// This is the first time seeing this category, so add it as zeroes.
					metric = &CategoryHostMetrics{}
					metrics[category.name] = metric
				}
				pss, ok := rollup.Rollup["Pss"]
				if !ok {
					return nil, errors.Errorf("smaps_rollup for process %d does not include Pss", rollup.Pid)
				}
				swapPss, ok := rollup.Rollup["SwapPss"]
				if !ok {
					return nil, errors.Errorf("smaps_rollup for process %d does not include SwapPss", rollup.Pid)
				}
				metric.Pss += pss
				metric.PssSwap += swapPss
				if rollup.Shared != nil {
					metric.PssSwap += rollup.Shared.SharedSwapPss
					metric.PssGuest += rollup.Shared.CrosvmGuestPss
				}
				// Only the first matching category should contain this process.
				break
			}
		}
	}
	return summary, nil
}

// ReportHostMetrics outputs a set of representative metrics
// into the supplied performance data dictionary.
func ReportHostMetrics(summary *HostSummary, p *perf.Values, suffix string) {
	for name, value := range summary.CategoryMetrics {
		p.Set(
			perf.Metric{
				Name:      fmt.Sprintf("%s%s_pss", name, suffix),
				Unit:      "MiB",
				Direction: perf.SmallerIsBetter,
			},
			float64(value.Pss)/KiBInMiB,
		)
		p.Set(
			perf.Metric{
				Name:      fmt.Sprintf("%s%s_pss_swap", name, suffix),
				Unit:      "MiB",
				Direction: perf.SmallerIsBetter,
			},
			float64(value.PssSwap)/KiBInMiB,
		)
		p.Set(
			perf.Metric{
				Name:      fmt.Sprintf("%s%s_pss_total", name, suffix),
				Unit:      "MiB",
				Direction: perf.SmallerIsBetter,
			},
			float64(value.Pss+value.PssSwap)/KiBInMiB,
		)
	}
}

// PerProcessSmapsRollup parses smaps and smaps_rollup information from every
// running process on both the ChromeOS (host) side and ARCVM (guest) side,
// without grouping. Fetching smaps_rollup from ARCVM requires adb root access.
func PerProcessSmapsRollup(ctx context.Context, hasArc bool) (*FullSmapsRollup, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get all processes")
	}

	// Separate host processes started by each crosvm instance.
	crosvmRollups := make(map[int32]*CrosVMSmapsRollup)

	// Regex to extract tag of crosvm.
	crosvmTagRegex := regexp.MustCompile(`--syslog-tag (\S+)\(\d+\)`)

	// Generate a map of processes and their parent PIDs so that we can separate
	// crosvm child processes from other host processes. Also generate a map of
	// processes names (/proc/*/comm).
	pPidMap := make(map[int32]int32)
	nameMap := make(map[int32]string)
	foundARCVMPid := false
	for _, p := range processes {
		pPid, err := p.Ppid()
		if err != nil {
			continue
		}
		pPidMap[p.Pid] = pPid

		commOut, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", p.Pid))
		if err != nil {
			continue
		}
		name := strings.Trim(string(commOut), "\n\x00")
		nameMap[p.Pid] = name

		if name == "crosvm" {
			cmdline, err := p.Cmdline()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get cmdline for crosvm process")
			}
			m := crosvmTagRegex.FindStringSubmatch(cmdline)
			if len(m) != 2 {
				return nil, errors.Errorf("failed to extract crosvm tag from cmdline %s", cmdline)
			}
			if m[1] == "ARCVM" {
				foundARCVMPid = true
			}
			crosvmRollups[p.Pid] = &CrosVMSmapsRollup{
				CrosVMPid: p.Pid,
				Tag:       m[1],
				Rollups:   []*NamedSmapsRollup{},
			}
		}
	}
	if hasArc && !foundARCVMPid {
		return nil, errors.New("unable to find crosvm process for ARCVM")
	}

	sharedInfoMap, err := makeSharedInfoMap(ctx, processes)
	if err != nil {
		return nil, err
	}

	hostRollups, err := smapsRollups(ctx, processes, sharedInfoMap, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get smaps_rollup on host")
	}

	result := &FullSmapsRollup{
		HostRollups:   []*NamedSmapsRollup{},
		CrosVMRollups: []*CrosVMSmapsRollup{},
		ARCVMRollups:  []*NamedSmapsRollup{},
	}
	for _, r := range hostRollups {
		// Replace process full cmdline with `/proc/*/comm` so that crosvm
		// processes will be separated by their functions (e.g. `pcivirtio-fs`
		// and `pcivirtio-net` instead of `crosvm`).
		name, ok := nameMap[r.Pid]
		if !ok {
			return nil, errors.Errorf("cannot find pid %d in process name map", r.Pid)
		}
		r.Command = name
		pPid, ok := pPidMap[r.Pid]
		if !ok {
			continue
		}

		// Store the process separately if it is created by crosvm.
		var crosvmRollup *CrosVMSmapsRollup
		if r, ok := crosvmRollups[r.Pid]; ok {
			crosvmRollup = r
		}
		if r, ok := crosvmRollups[pPid]; ok {
			crosvmRollup = r
		}
		if crosvmRollup != nil {
			crosvmRollup.Rollups = append(crosvmRollup.Rollups, r)
		} else {
			result.HostRollups = append(result.HostRollups, r)
		}
	}

	for _, r := range crosvmRollups {
		result.CrosVMRollups = append(result.CrosVMRollups, r)
	}

	// Fill in ARCVMRollups if hasArc is true.
	if hasArc {
		arcvmRollups, err := arcSmapsRollups(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get smaps_rollup from ARCVM")
		}
		result.ARCVMRollups = arcvmRollups
	}

	return result, nil
}

// arcSmapsRollups returns a NamedSmapsRollup for every process in ARCVM.
// Sizes are in kilobytes.
func arcSmapsRollups(ctx context.Context) ([]*NamedSmapsRollup, error) {
	d, err := rootArcADBDevice(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get root ADB device")
	}

	var procFiles []string
	// On slow devices "adb shell" is not immediately usable after adbd is
	// restarted as root.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		procFiles, err = d.ListContents(ctx, "/proc")
		return err
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 3 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to list /proc via ADB")
	}

	var result []*NamedSmapsRollup
	for _, procFile := range procFiles {
		pid, err := strconv.ParseInt(procFile, 10, 31)
		if err != nil {
			continue
		}
		// We're racing with this process potentially exiting, so just
		// ignore errors and don't generate a NamesSmapsRollup if we fail to
		// read anything from proc.
		smapsData, err := d.ReadFile(ctx, fmt.Sprintf("/proc/%d/smaps_rollup", pid))
		if err != nil {
			// Not all processes have a smaps_rollup, this process may have
			// exited.
			continue
		}
		if len(smapsData) == 0 {
			// On some processes, smaps_rollup exists but is empty.
			continue
		}
		rollup, err := NewSmapsRollup(smapsData)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse /proc/%d/smaps_rollup", pid)
		}

		// Extract and clean up process name from cmdline.
		cmdline, err := d.ReadFile(ctx, fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		name := strings.ReplaceAll(string(cmdline), "\x00", " ")
		name = strings.Split(name, " ")[0]
		name = filepath.Base(name)

		result = append(result, &NamedSmapsRollup{
			Command: name,
			Pid:     int32(pid),
			Rollup:  rollup,
			Shared:  nil,
		})
	}

	return result, nil
}

// rootArcADBDevice gets the ADB device with root permission for the ARCVM.
func rootArcADBDevice(ctx context.Context) (*adb.Device, error) {
	adbDevices, err := adb.Devices(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get ADB devices")
	}
	var adbDevice *adb.Device
	for _, d := range adbDevices {
		if strings.HasPrefix(d.Serial, "emulator-") {
			adbDevice = d
			break
		}
	}
	if adbDevice == nil {
		return nil, errors.New("unable to find ADB device for ARCVM")
	}

	if err := adbDevice.Root(ctx); err != nil {
		return nil, errors.Wrap(err, "unable to get root")
	}
	if err := adbDevice.WaitForState(ctx, adb.StateDevice, 30*time.Second); err != nil {
		return nil, errors.Wrap(err, "wait for state failed")
	}

	return adbDevice, nil
}

// SaveRawSmaps saves a zip archive containing a copy of `/proc/{pid}/smaps` for
// all processes in `smapsRollup`, which can be used to calculate memory used by
// individual libraries. Additionally, it also collects `cmdline` and `comm` to
// identify processes. If `hasArc“ is true, it will save smaps files from ARC
// as well.
func SaveRawSmaps(ctx context.Context, outdir, suffix string, smapsRollup *FullSmapsRollup, hasArc bool) error {
	filesToCollect := []string{"smaps", "cmdline", "comm"}

	zipPath := path.Join(outdir, fmt.Sprintf("raw_smaps%s.zip", suffix))
	zipFile, err := os.OpenFile(zipPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to open zip file for write")
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Merge all host processes into a single slice for iterating.
	var allHostRollups []*NamedSmapsRollup
	allHostRollups = append(allHostRollups, smapsRollup.HostRollups...)
	for _, r := range smapsRollup.CrosVMRollups {
		allHostRollups = append(allHostRollups, r.Rollups...)
	}

	for _, s := range allHostRollups {
		for _, f := range filesToCollect {
			smapsPath := fmt.Sprintf("/proc/%d/%s", s.Pid, f)
			smapsContent, err := os.ReadFile(smapsPath)
			if err != nil {
				testing.ContextLogf(ctx, "Failed to read %s: %v", smapsPath, err)
				continue
			}

			fileName := fmt.Sprintf("host/%d/%s", s.Pid, f)
			if err := writeDataToZip(zipWriter, fileName, smapsContent); err != nil {
				return errors.Wrap(err, "failed to write data to zip")
			}
		}
	}

	if !hasArc {
		return nil
	}

	d, err := rootArcADBDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to get root ADB device")
	}

	for _, s := range smapsRollup.ARCVMRollups {
		for _, f := range filesToCollect {
			smapsPath := fmt.Sprintf("/proc/%d/%s", s.Pid, f)
			smapsContent, err := d.ReadFile(ctx, smapsPath)
			if err != nil {
				testing.ContextLogf(ctx, "Failed to read %s from ARC: %v", smapsPath, err)
				continue
			}

			fileName := fmt.Sprintf("arc/%d/%s", s.Pid, f)
			if err := writeDataToZip(zipWriter, fileName, smapsContent); err != nil {
				return errors.Wrap(err, "failed to write data to zip")
			}
		}
	}

	return nil
}

// writeDataToZip writes data stored in a byte slice into a new file in a zip
// archive.
func writeDataToZip(zip *zip.Writer, fileName string, data []byte) error {
	fileInZip, err := zip.Create(fileName)
	if err != nil {
		return errors.Wrapf(err, "failed to create %s in zip", fileName)
	}

	if _, err := fileInZip.Write(data); err != nil {
		return errors.Wrapf(err, "failed to write %s", fileName)
	}

	return nil
}
