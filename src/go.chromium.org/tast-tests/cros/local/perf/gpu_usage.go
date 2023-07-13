// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"math"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/async"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// This file implements the GPUUsageDataSource performance timeline data source
// For the GPU usage info. See https://docs.kernel.org/gpu/drm-usage-stats.html.
// The implementation is very similar to gputop, a command line tool to track
// GPU usage. Please see https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/third_party/igt-gpu-tools/tools/gputop.c.
//
// According to the Linux Kernel documentation (https://www.kernel.org/doc/html/v5.15/gpu/drm-internals.html),
// DRM (Direct Rendering Manager) is a subsystem of the Linux kernel that
// provides an interface for applications to use the GPU for 3D rendering, video
// decoding, and display management. DRM assigns a device number to each driver
// that registers with it, and a device number to each device instance that the
// driver controls. The major and minor numbers are used to identify the driver
// and the device by the kernel and the userspace applications.
//
// The GPU usage is tracked on a per DRM (identified by the DRM minor number),
// per process, and per Engine (or memory kind) basis, as illustrated below:
//
// GPU Utilization
//   |-- DRM 1
//   |    |--Process 1
//   |    |      |-- DRM Engine 1 Usage
//   |    |      |-- DRM Engine 2 Usage
//   |    |      |-- ...
//   |    |
//   |    |--Process 2
//   |    |      |-- DRM Engine 1 Usage
//   |    |      |-- DRM Engine 2 Usage
//   |    |      |-- ...
//   |    |--...
//   |
//   |-- DRM 2
//   |    |--Process 1
//   |    |      |-- DRM Engine 1 Usage
//   |    |      |-- DRM Engine 2 Usage
//   |    |      |-- ...
//   |    |
//   |    |--Process 3
//   |    |      |-- DRM Engine 1 Usage
//   |    |      |-- DRM Engine 2 Usage
//   |    |      |-- ...
//   |    |--...
//   |
//   |-- ...
//
// GPU Memory
//   |-- DRM 1
//   |    |--Process 1
//   |    |      |-- Memory Kind 1 Usage
//   |    |      |-- Memory Kind 2 Usage
//   |    |      |-- ...
//   |    |
//   |    |--Process 2
//   |    |      |-- Memory Kind 1 Usage
//   |    |      |-- Memory Kind 2 Usage
//   |    |      |-- ...
//   |    |--...
//   |
//   |-- DRM 2
//   |    |--Process 1
//   |    |      |-- Memory Kind 1 Usage
//   |    |      |-- Memory Kind 2 Usage
//   |    |      |-- ...
//   |    |
//   |    |--Process 3
//   |    |      |-- Memory Kind 1 Usage
//   |    |      |-- Memory Kind 2 Usage
//   |    |      |-- ...
//   |    |--...
//   |
//   |-- ...
//
// The GPU usage information is read from the fdinfo file. Support for fdinfo
// based per-process GPU busyness and memory usage in ChromeOS is tracked by
// - b/278110760 Add support for fdinfo based per-process GPU busyness for
//   GuC-based platforms
// - b/278115755 Add support for fdinfo based GPU memory stats in 5.10 and 5.15
//
// The GPUUsageDataSource implemented here will work on Linux Kernel 5.10 and
// later versions after all changes for the above two bugs are merged.

const (
	// Log patterns for reading GPU usage from file.
	logUtilizationUnitMissing = "GPU utilization unit is not given"
	logClientIDMissing        = "No DRM client id is found"

	// Log patternsfor reading process fdinfo.
	logNotDir        = "The path is not a directory"
	logDirNotExist   = "The path does not exist"
	logStatFileErr   = "Cannot get file stat"
	logNoLongerExist = "File is no longer existent"
	logFdNotExist    = "Corresponding FD file does not exist"

	// Log patterns for utilization adjustment.
	logUtilSampleAdjustment = "Adjust utilization for values greater than one"
	logUtilTotalAdjustment  = "Adjust total utilization for values greater than one"
)

// Perf timeline does snapshot repeatly with a short interval. To reduce the
// log volume, we design the log / logs data structures so each pattern of log
// will be printed only once.
type log struct {
	count   int    // Occurrence number for a certain log pattern.
	example string // Example of the log for the pattern.
}

// logs keeps track of log info keyed by its pattern string.
type logs map[string]*log

// addLog adds a log into the logs map based on its pattern.
func addLog(ls logs, pattern string, l *log) {
	if info, ok := ls[pattern]; ok {
		info.count += l.count
	} else {
		ls[pattern] = l
	}
}

// gpuUtilization tracks the per engine GPU utilization. It is keyed by the GPU
// engine name, such as "render", "copy", "video", and "video-enhance".
type gpuUtilization map[string]float64

// gpuMemory tracks the GPU memory of different kinds.
// It is keyed by the memory kind, such as "total", "shared", "active".
type gpuMemory map[string]float64

// drmUsage contains the per GPU DRM usage info. A GPU DRM is identified
// by the minor part of the Linux device number.
type drmUsage struct {
	drmMinor uint32 // Minor part of DRM device number.

	// Keep unique names for process, engine, and memory kind.
	allProcesses   map[string]bool
	allEngines     map[string]bool
	allMemoryKinds map[string]bool

	// Last GPU utilization keyed by the process. Used to calculate the
	// utilization percentage between current and last snapshot.
	lastUtilization map[string]gpuUtilization

	// Utilization and memory data samples. Elements are keyed by the process.
	utilizationSamples []map[string]gpuUtilization
	memorySamples      []map[string]gpuMemory
}

// newDrmUsage returns an instance of drmUsage.
func newDrmUsage(minor uint32) *drmUsage {
	return &drmUsage{
		drmMinor:        minor,
		lastUtilization: make(map[string]gpuUtilization),
		allProcesses:    make(map[string]bool),
		allEngines:      make(map[string]bool),
		allMemoryKinds:  make(map[string]bool),
	}
}

// GPUUsageDataSource is performance timeline data source to get GPU utilization
// and memory usage from the system proc fdinfo.
type GPUUsageDataSource struct {
	prefix       string
	intervalName string

	skip     bool      // Whether GPU collection should be skipped.
	lastTime time.Time // Last sampling time.

	// drms is the per DRM GPU usage keyed by the DRM minor.
	drms map[uint32]*drmUsage
	// logs keeps track of logs happened during the collection.
	logs logs
	// snapshotTime keeps track of the snapshot duration.
	snapshotTime []time.Duration
	// procNums tracks the number of processes analyzed for each snapshot.
	procNums []int
	// fileNums tracks the number of files analyzed for each snapshot.
	fileNums []int
}

// Assert that GPUInfoSource can be used in perf.Timeline.
var _ perf.TimelineDatasource = &GPUUsageDataSource{}

// NewGPUUsageDataSource creates an instance of GPUUsageDataSource.
func NewGPUUsageDataSource() *GPUUsageDataSource {
	return &GPUUsageDataSource{
		drms: make(map[uint32]*drmUsage),
		logs: make(logs),
	}
}

// Close does nothing.
func (ds *GPUUsageDataSource) Close() {
	return
}

// Setup implements perf.TimelineDatasource.Setup.
func (ds *GPUUsageDataSource) Setup(ctx context.Context, prefix, intervalName string) error {
	ds.prefix = prefix
	ds.intervalName = intervalName

	kernelVersion, _, err := sysutil.KernelVersionAndArch()
	if err != nil {
		return errors.Wrap(err, "failed to get kernel version and arch")
	}
	// DUTs with kernel version 5.10 or later support GPU usage fdinfo at this
	// time.
	// TODO (b/277656113): Update this to 5.10 after b/278110760 and b/278115755
	// are back ported to kernel 5.10.
	if !kernelVersion.IsOrLater(5, 15) {
		testing.ContextLog(ctx, "The GPU usage metrics will not be collected due to unsupported kernel version: ", kernelVersion)
		ds.skip = true
	}
	return nil
}

// Start implements perf.TimelineDatasource.Start.
func (ds *GPUUsageDataSource) Start(ctx context.Context) error {
	if ds.skip {
		return nil
	}
	testing.ContextLog(ctx, "Start tracking GPU usage metrics")
	// Collect the initial GPU usage info of each DRM.
	drms, samplingTime, err := ds.getGPUInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to collect initial metrics")
	}

	// Save GPU usage for each DRM into ds.drms.
	for minor, drm := range drms {
		if ds.drms[minor] == nil {
			ds.drms[minor] = newDrmUsage(minor)
		}
		drmSaved := ds.drms[minor]

		// Loop through all the processes having GPU utilization for the DRM.
		for proc, perProcUtil := range drm.utilizationSamples[0] {
			// Save the processes and engines related to this DRM.
			drmSaved.allProcesses[proc] = true
			for engine := range perProcUtil {
				drmSaved.allEngines[engine] = true
			}
		}
		// Keep the GPU utilization time so utilization percentage can be
		// obtained during next snapshot.
		drmSaved.lastUtilization = drm.utilizationSamples[0]

		// Loop through all the processes having GPU memory usage for the DRM.
		for proc, perProcMemory := range drm.memorySamples[0] {
			// Save the processes and memory kind related to this DRM.
			drmSaved.allProcesses[proc] = true
			for kind := range perProcMemory {
				drmSaved.allMemoryKinds[kind] = true
			}
		}
	}
	ds.lastTime = samplingTime
	return nil
}

// Snapshot implements perf.TimelineDatasource.Snapshot.
func (ds *GPUUsageDataSource) Snapshot(ctx context.Context, values *perf.Values) error {
	if ds.skip {
		return nil
	}
	startTime := time.Now()
	// Collect the current GPU usage info of each DRM.
	drms, samplingTime, err := ds.getGPUInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get GPU usage info")
	}

	samplingDuration := samplingTime.Sub(ds.lastTime)

	// Loop through all the processes having GPU utilization for the DRM
	// during this snapshot.
	for minor, drm := range drms {
		if ds.drms[minor] == nil {
			ds.drms[minor] = newDrmUsage(minor)
		}
		drmSaved := ds.drms[minor]

		// Get utilization percentage from the lastUtilization, keyed by the
		// process.
		utilDiffs := make(map[string]gpuUtilization)
		// Loop through all the processes having GPU utilization for the DRM.
		for proc, perProcUtil := range drm.utilizationSamples[0] {
			drmSaved.allProcesses[proc] = true
			utilDiffs[proc] = make(gpuUtilization)
			for engine, perEngineUtil := range perProcUtil {
				drmSaved.allEngines[engine] = true
				lastValue := 0.0
				if lastProcUtil, ok := drmSaved.lastUtilization[proc]; ok {
					lastValue = lastProcUtil[engine]
				}
				// Utilization percentage is calculated from the usage time
				// divided by the sampling duration.
				util := (perEngineUtil - lastValue) / float64(samplingDuration) * 100
				// Ensure utilization is not over 100%.
				if util > 100.0 {
					addLog(ds.logs, logUtilSampleAdjustment, &log{1,
						fmt.Sprintf("%s: got %f for proc %s and engine %s", logUtilSampleAdjustment, util, proc, engine)})
					util = 100.0
				}
				utilDiffs[proc][engine] = util
			}
		}
		// Keep the GPU utilization time so utilization percentage can be
		// obtained during next snapshot.
		drmSaved.lastUtilization = drm.utilizationSamples[0]

		// Loop through all the processes having GPU memory usage for the DRM.
		for proc, perProcMemory := range drm.memorySamples[0] {
			drmSaved.allProcesses[proc] = true
			for kind := range perProcMemory {
				drmSaved.allMemoryKinds[kind] = true
			}
		}
		// Attach the samples of the current snapshot to the saved DRM usage
		// sample slice.
		drmSaved.utilizationSamples = append(drmSaved.utilizationSamples, utilDiffs)
		drmSaved.memorySamples = append(drmSaved.memorySamples, drm.memorySamples[0])
	}

	ds.lastTime = samplingTime
	ds.snapshotTime = append(ds.snapshotTime, time.Now().Sub(startTime))
	return nil
}

// Stop saves the collected GPU usage information to the perf values.
func (ds *GPUUsageDataSource) Stop(ctx context.Context, values *perf.Values) error {
	if ds.skip {
		return nil
	}
	testing.ContextLog(ctx, "Stop tracking GPU usage metrics")

	// Loop through all the saved GPU usage on a per DRM basis.
	for minor, drm := range ds.drms {
		// Total GPU utilization by all processes for the DRM, keyed by the
		// engine.
		totalUtilization := make(map[string][]float64)
		for engine := range drm.allEngines {
			totalUtilization[engine] = make([]float64, len(drm.utilizationSamples))
		}
		// Total GPU memory by all processes for the DRM, keyed by the memory
		// kind.
		totalMemory := make(map[string][]float64)
		for kind := range drm.allMemoryKinds {
			totalMemory[kind] = make([]float64, len(drm.memorySamples))
		}
		// Flags to track whether the DRM has been used during the test.
		drmHasUtilization := false
		drmHasMemory := false
		// Loop through all the processes that have been recorded for the DRM.
		for proc := range drm.allProcesses {
			// Map used to store the utilization samples on a per engine basis.
			processUtilization := make(map[string][]float64)
			for engine := range drm.allEngines {
				processUtilization[engine] = nil
			}
			// Map used to store the memory samples on a per memory kind basis.
			processMemory := make(map[string][]float64)
			for kind := range drm.allMemoryKinds {
				processMemory[kind] = nil
			}

			// Flags to track whether the process used the GPU during the test.
			processHasEngineUtilization := make(map[string]bool)
			processHasKindMemory := make(map[string]bool)
			// Loop through all GPU utilization samples.
			for i, samples := range drm.utilizationSamples {
				procSamples, ok := samples[proc]
				if !ok {
					// Samples are not captured for this proc in this snapshot.
					// Use zero values.
					for engine := range drm.allEngines {
						processUtilization[engine] = append(processUtilization[engine], 0.0)
						totalUtilization[engine][i] += 0.0
					}
					continue
				}
				for engine := range drm.allEngines {
					processUtilization[engine] = append(processUtilization[engine], procSamples[engine])
					// Accumulate the tolat utilization for this engine.
					util := totalUtilization[engine][i] + procSamples[engine]
					// Ensure total engine utilization is not over 100%.
					if util > 100.0 {
						addLog(ds.logs, logUtilTotalAdjustment, &log{1,
							fmt.Sprintf("%s: got %f for engine %s", logUtilTotalAdjustment, util, engine)})
						util = 100.0
					}
					totalUtilization[engine][i] = util
					if procSamples[engine] > 0 {
						drmHasUtilization = true
						processHasEngineUtilization[engine] = true
					}
				}
			}
			// Loop through all GPU memory samples.
			for i, samples := range drm.memorySamples {
				procSamples, ok := samples[proc]
				if !ok {
					// Samples are not captured for this proc in this snapshot.
					// Use zero values.
					for kind := range drm.allMemoryKinds {
						processMemory[kind] = append(processMemory[kind], 0.0)
						totalMemory[kind][i] += 0.0
					}
					continue
				}
				for kind := range drm.allMemoryKinds {
					processMemory[kind] = append(processMemory[kind], procSamples[kind])
					// Accumulate the tolat memory for this memory kind.
					totalMemory[kind][i] += procSamples[kind]
					if procSamples[kind] > 0 {
						drmHasMemory = true
						processHasKindMemory[kind] = true
					}
				}
			}
			// Add per-process metric values.
			for engine := range drm.allEngines {
				if !processHasEngineUtilization[engine] {
					continue
				}
				values.Set(perf.Metric{
					Name:      fmt.Sprintf("%sGPU.Usage.DRM%d.Utilization.%s.%s", ds.prefix, minor, engine, proc),
					Unit:      "percent",
					Direction: perf.SmallerIsBetter,
					Multiple:  true,
					Interval:  ds.intervalName,
				}, processUtilization[engine]...)

			}
			for kind := range drm.allMemoryKinds {
				if !processHasKindMemory[kind] {
					continue
				}
				values.Set(perf.Metric{
					Name:      fmt.Sprintf("%sGPU.Usage.DRM%d.Memory.%s.%s", ds.prefix, minor, kind, proc),
					Unit:      "KiB",
					Direction: perf.SmallerIsBetter,
					Multiple:  true,
					Interval:  ds.intervalName,
				}, processMemory[kind]...)
			}
		}
		// Add total metric values.
		if drmHasUtilization {
			for engine := range drm.allEngines {
				values.Set(perf.Metric{
					Name:      fmt.Sprintf("%sGPU.Usage.DRM%d.Utilization.%s", ds.prefix, minor, engine),
					Unit:      "percent",
					Direction: perf.SmallerIsBetter,
					Multiple:  true,
					Interval:  ds.intervalName,
				}, totalUtilization[engine]...)
			}
		}
		if drmHasMemory {
			for kind := range drm.allMemoryKinds {
				values.Set(perf.Metric{
					Name:      fmt.Sprintf("%sGPU.Usage.DRM%d.Memory.%s", ds.prefix, minor, kind),
					Unit:      "KiB",
					Direction: perf.SmallerIsBetter,
					Multiple:  true,
					Interval:  ds.intervalName,
				}, totalMemory[kind]...)
			}
		}
	}

	for pattern, log := range ds.logs {
		testing.ContextLogf(ctx, "GPU usage log pattern: %q; occurrence number: %d; example: %s", pattern, log.count, log.example)
	}
	snapshots := float64(len(ds.snapshotTime))
	if snapshots > 0 {
		var totalDuration float64
		var totalProc int
		var totalFile int
		for i, d := range ds.snapshotTime {
			totalDuration += float64(d / time.Millisecond)
			totalProc += ds.procNums[i]
			totalFile += ds.fileNums[i]
		}
		// Print the snapshot statistics so the performance of the GPU usage
		// collection can be evaluated.
		testing.ContextLogf(ctx,
			"GPU usage average time of doing snapshot: %f ms; average number of processes and files analyzed per snapshot: %f, %f",
			totalDuration/snapshots, float64(totalProc)/snapshots, float64(totalFile)/snapshots)

	}
	return nil
}

const (
	procDirPath = "/proc"
	// Device number major part for DRM GPU devices. 226 indicates a DRM device.
	drmMajor uint32 = 226
)

// gpuInfo contains the info read from the fdinfo file that shows the GPU usage.
type gpuInfo struct {
	minor        uint32    // The DRM minor identifying which DRM is used.
	file         string    // The file path where the usage info is read from.
	samplingTime time.Time // When the GPU usage info is read.
	drmClient    string    // The id of the DRM client that is using the GPU.

	utilization gpuUtilization // GPU Utilization.
	memory      gpuMemory      // Memory utilization.
	logs        logs           // Logs occurred when reading info from this file.
}

// procInfo contains the GPU usage info read for each process.
type procInfo struct {
	useGPU bool   // Whether the process has used the GPU.
	pid    string // Process id.
	name   string // Process name.

	gpuUsage []*gpuInfo // Multiple instances of GPU usage info of the process.
	logs     logs       // Logs occurred when reading GPU info for this process.
	fileNum  int        // Number of fdinfo files analyzed.
}

// getGPUInfo gets the GPU usage information for all the processes that use
// GPU on a per DRM basis.
// The sampling time is also returned.
func (ds *GPUUsageDataSource) getGPUInfo(ctx context.Context) (map[uint32]*drmUsage, time.Time, error) {
	processes, err := ds.gpuClientProcesses(ctx)
	if err != nil {
		return nil, time.Time{}, errors.Wrap(err, "failed to obtain GPU client processes")
	}

	var procNames []string
	for proc := range processes {
		procNames = append(procNames, proc)
	}
	// Sort by proc Name, so we can ignore processes by the
	// order in case their DRM clients are the same.
	sort.Strings(procNames)

	drms := make(map[uint32]*drmUsage)

	// A map for checking unique DRM client IDs that a process uses.
	// Key is the combination of the DRM device number minor and DRM Client ID.
	uniqueDrmClientIDs := make(map[string]bool)
	// Find the latest reading time as the sampling time.
	var samplingTime time.Time

	for _, proc := range procNames {
		gpuResults := processes[proc]
		for _, r := range gpuResults {
			minor := r.minor
			drm, ok := drms[minor]
			if !ok {
				drm = &drmUsage{
					drmMinor: minor,
					// Only use the first element to store GPU utilization and memory data
					// for the DRM for the current snapshot.
					utilizationSamples: []map[string]gpuUtilization{make(map[string]gpuUtilization)},
					memorySamples:      []map[string]gpuMemory{make(map[string]gpuMemory)},
				}
				drms[minor] = drm
			}
			gu := drm.utilizationSamples[0]
			gm := drm.memorySamples[0]

			key := fmt.Sprintf("%d_%s", minor, r.drmClient)
			if uniqueDrmClientIDs[key] {
				// Ignore the duplicate DRM client.
				continue
			}
			uniqueDrmClientIDs[key] = true
			// Add the info into the result map.
			if gu[proc] == nil {
				gu[proc] = r.utilization
			} else {
				// Add up to the existing utilization.
				for engine, num := range r.utilization {
					gu[proc][engine] += num
				}
			}
			if gm[proc] == nil {
				gm[proc] = r.memory
			} else {
				// Add up to the existing memory.
				for kind, num := range r.memory {
					gm[proc][kind] += num
				}
			}
			if samplingTime.Before(r.samplingTime) {
				samplingTime = r.samplingTime
			}
		}
	}

	return drms, samplingTime, nil
}

// gpuClientProcesses returns all the processes that use the GPU.
// A list of gpuInfo for each process will be returned.
func (ds *GPUUsageDataSource) gpuClientProcesses(ctx context.Context) (map[string][]*gpuInfo, error) {
	procDir, err := ioutil.ReadDir(procDirPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s directory", procDirPath)
	}

	procNum := len(procDir)
	// Maximum number of goroutines for concurrent processing. The number
	// balances the snapshot time and potential goroutine overhead.
	maxConcurrency := 8
	// Split processes into groups. Each group will be handled by a goroutine.
	var concurrencyGroups [][]fs.FileInfo
	groupSize := int(math.Ceil(float64(procNum) / float64(maxConcurrency)))
	for i := 0; i < procNum; i += groupSize {
		end := i + groupSize
		if end > procNum {
			end = procNum
		}
		concurrencyGroups = append(concurrencyGroups, procDir[i:end])
	}
	// Make buffered channels so the goroutines can send info back
	// without being blocked.
	readResults := make(chan *procInfo, procNum)
	readErrs := make(chan error, procNum)

	var wg sync.WaitGroup
	wg.Add(len(concurrencyGroups))
	// Use goroutine to analyze processes concurrently.
	asyncRun := func(procGrp []fs.FileInfo) {
		async.Run(ctx, func(ctx context.Context) {
			analyzeProc(procGrp, readResults, readErrs, &wg)
		}, "analyzeProc")
	}
	for _, procGrp := range concurrencyGroups {
		asyncRun(procGrp)
	}
	wg.Wait()
	close(readResults)
	close(readErrs)

	if len(readErrs) > 0 {
		// Return with the first error.
		return nil, errors.Wrap(<-readErrs, "failed to analyze proc info")
	}

	processes := make(map[string][]*gpuInfo)

	fileNum := 0
	for r := range readResults {
		fileNum += r.fileNum
		for pattern, log := range r.logs {
			addLog(ds.logs, pattern, log)
		}
		if !r.useGPU {
			continue
		}
		// Use <process name>_<process id> as key.
		processes[fmt.Sprintf("%s_%s", r.name, r.pid)] = r.gpuUsage
	}
	ds.fileNums = append(ds.fileNums, fileNum)
	ds.procNums = append(ds.procNums, procNum)

	return processes, nil
}

// analyzeProc analyzes the files under each process and get the GPU usage info
// if it uses GPU.
func analyzeProc(procs []fs.FileInfo, results chan *procInfo, errs chan error, wg *sync.WaitGroup) {
	defer wg.Done()
	for _, proc := range procs {
		if !proc.IsDir() {
			continue
		}
		pid := proc.Name() // File name is the process id.
		// Matches directories with numeric names.
		match, _ := regexp.MatchString(`\d+`, pid)
		if !match {
			continue
		}
		result := &procInfo{pid: pid, logs: make(logs)}
		pidDirPath := path.Join(procDirPath, pid)

		// Read "stat" file to get the process name.
		content, err := ioutil.ReadFile(path.Join(pidDirPath, "stat"))
		if err != nil {
			continue
		}
		// The stat file has content like:
		// 25013 (chrome) S 24952 ...
		// We'll get the process name in the parentheses.
		reg := regexp.MustCompile(`^\d+\s+\((.*)\)`)
		matches := reg.FindStringSubmatch(string(content))
		if matches == nil {
			continue
		}
		result.name = matches[1]

		fdinfoDirPath := path.Join(pidDirPath, "fdinfo")
		fdinfoDirStat, err := os.Stat(fdinfoDirPath)
		if err != nil {
			// Check if the "fdinfo" directory is under the process directory.
			if os.IsNotExist(err) {
				addLog(result.logs, logDirNotExist,
					&log{1, fmt.Sprintf("%s for %s: %v", logDirNotExist, fdinfoDirPath, err)})
				results <- result
				continue
			}
			errs <- errors.Wrapf(err, "failed to get dir stat info of %s", fdinfoDirPath)
			return
		}
		if !fdinfoDirStat.IsDir() {
			addLog(result.logs, logNotDir,
				&log{1, fmt.Sprintf("%s for %s", logNotDir, fdinfoDirPath)})
			results <- result
			continue
		}
		// Get all files under <pid>/fdinfo/.
		fdinfoDir, err := ioutil.ReadDir(fdinfoDirPath)
		if err != nil {
			errs <- errors.Wrapf(err, "failed to read dir of %s", fdinfoDirPath)
			return
		}

		result.fileNum = len(fdinfoDir)
		for _, f := range fdinfoDir {
			r, err := analyzeFile(pidDirPath, f)
			if err != nil {
				errs <- errors.Wrap(err, "failed to analyze file")
				return
			}
			for pattern, log := range r.logs {
				addLog(result.logs, pattern, log)
			}
			if r.drmClient == "" {
				// Not a DRM client.
				continue
			}
			result.useGPU = true
			result.gpuUsage = append(result.gpuUsage, r)
		}
		results <- result
	}
}

// analyzeFile analyzes the fdinfo file and its corresponding fd file, and get
// the GPU usage from the fdinfo file.
func analyzeFile(pidDirPath string, f fs.FileInfo) (*gpuInfo, error) {
	result := &gpuInfo{
		utilization: make(gpuUtilization),
		memory:      make(gpuMemory),
		logs:        make(logs),
	}

	if !f.Mode().IsRegular() {
		return result, nil
	}
	// Check corresponding fd file to see if it is a DRM client.
	fdFilePath := path.Join(pidDirPath, "fd", f.Name())
	var statT unix.Stat_t
	err := unix.Stat(fdFilePath, &statT)
	if err != nil {
		if os.IsNotExist(err) {
			addLog(result.logs, logFdNotExist,
				&log{1, fmt.Sprintf("%s for %s: %v", logFdNotExist, fdFilePath, err)})
			return result, nil
		}
		addLog(result.logs, logStatFileErr,
			&log{1, fmt.Sprintf("%s for %s: %v", logStatFileErr, fdFilePath, err)})
		return result, nil
	}

	// DRM client must be a character device. Make sure the CharDevice bit is
	// set in the file's mode.
	if (statT.Mode & unix.S_IFCHR) != unix.S_IFCHR {
		return result, nil
	}

	rdev := statT.Rdev
	if unix.Major(rdev) != drmMajor {
		return result, nil
	}

	result.minor = unix.Minor(rdev)
	result.file = path.Join(pidDirPath, "fdinfo", f.Name())
	if err := gpuFdinfo(result); err != nil {
		return nil, errors.Wrap(err, "failed to get GPU fdinfo")
	}
	return result, nil
}

// gpuFdinfo reads the GPU information from fdinfo file.
func gpuFdinfo(result *gpuInfo) error {
	// An example of the fdinfo file is:
	//
	// kohaku-rev6 ~ # cat /proc/10236/fdinfo/46
	// pos:	0
	// flags:	0100002
	// mnt_id:	22
	// ino:	125
	// drm-driver:	i915
	// drm-client-id:	22
	// drm-pdev:	0000:00:02.0
	// drm-engine-render:	0 ns
	// drm-engine-copy:	0 ns
	// drm-engine-video:	0 ns
	// drm-engine-video-enhance:	0 ns
	// drm-total-memory:	102 MiB
	// drm-shared-memory:	102 MiB
	// drm-active-memory:	0
	engineRegexp := regexp.MustCompile(`^drm-engine-(.+):\s*(\d+)\s*(.*)$`)
	memoryRegexp1 := regexp.MustCompile(`^drm-(.+)-memory:\s*(\d+)\s*(.*)$`)
	memoryRegexp2 := regexp.MustCompile(`^drm-memory-(.+):\s*(\d+)\s*(.*)$`)
	clientIDRegexp := regexp.MustCompile(`^drm-client-id:\s*(.+)$`)

	path := result.file
	f, err := os.Open(path)
	if err != nil {
		// File might have already been deleted.
		if os.IsNotExist(err) {
			addLog(result.logs, logNoLongerExist,
				&log{1, fmt.Sprintf("%s for %s: %v", logNoLongerExist, path, err)})
			return nil
		}
		return errors.Wrapf(err, "failed to open fdinfo file %s", path)
	}
	defer f.Close()

	var clientID string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineText := strings.TrimSpace(scanner.Text())
		engineMatches := engineRegexp.FindStringSubmatch(lineText)
		if engineMatches != nil {
			engine := engineMatches[1]
			usage := engineMatches[2]
			unit := engineMatches[3]

			value, err := strconv.ParseFloat(usage, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse value %s in line %q in %s", usage, lineText, path)
			}
			if value != 0.0 {
				if unit == "" {
					// Unit missing has been found in certain models, such as
					// redrix/brya. Log it.
					addLog(result.logs, logUtilizationUnitMissing,
						&log{1, fmt.Sprintf("%s in line %q in %s; assume it is ns", logUtilizationUnitMissing, lineText, path)})

				} else if unit != "ns" {
					return errors.Errorf("utilization unit %q in line %q is not ns in %s", unit, lineText, path)
				}
			}
			result.utilization[engine] = value
			continue
		}

		memoryMatches := memoryRegexp1.FindStringSubmatch(lineText)
		if memoryMatches == nil {
			memoryMatches = memoryRegexp2.FindStringSubmatch(lineText)
		}
		if memoryMatches != nil {
			kind := memoryMatches[1]
			usage := memoryMatches[2]
			unit := memoryMatches[3]

			value, err := strconv.ParseFloat(usage, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse value %s in line %q in %s", usage, lineText, path)
			}
			if value != 0.0 {
				if unit == "" {
					return errors.Errorf("memory has no unit in line %q in %s", lineText, path)
				}
				if unit == "MiB" {
					value *= 1024
				} else if unit != "KiB" {
					return errors.Errorf("memory has unsupported unit %s in line %q in %s", unit, lineText, path)
				}
			}
			result.memory[kind] = value
			continue
		}

		clientIDMatches := clientIDRegexp.FindStringSubmatch(lineText)
		if clientIDMatches != nil {
			clientID = clientIDMatches[1]
		}
	}

	if clientID == "" {
		// Found missing client ID in certain models, such as steelix/corsola.
		addLog(result.logs, logClientIDMissing,
			&log{1, fmt.Sprintf("%s in %s", logClientIDMissing, path)})

	}
	result.drmClient = clientID
	result.samplingTime = time.Now()

	return nil
}
