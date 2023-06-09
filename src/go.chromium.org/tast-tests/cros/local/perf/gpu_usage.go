// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"time"

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
	logSysStatConvertion = "Failed to convert fdinfo Sys() to Stat_t"
	logNotDir            = "The path is not a directory"
	logDirNotExist       = "The path does not exist"
	logStatFileErr       = "Cannot get file stat"
	logNoLongerExist     = "File is no longer existent"
	logFdNotExist        = "Corresponding FD file does not exist"

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
	// TODO (b/277656113): Remove the following line after this serial of CLs
	// are all merged.
	ds.skip = true // Disable it temporarily until this feature is ready.
	return nil
}

// Start implements perf.TimelineDatasource.Start.
func (ds *GPUUsageDataSource) Start(ctx context.Context) error {
	if ds.skip {
		return nil
	}
	testing.ContextLog(ctx, "Start tracking GPU usage metrics")
	// TODO (b/277656113): Collect the initial GPU usage info and save to ds.drms.

	return nil
}

// Snapshot implements perf.TimelineDatasource.Snapshot.
func (ds *GPUUsageDataSource) Snapshot(ctx context.Context, values *perf.Values) error {
	if ds.skip {
		return nil
	}
	startTime := time.Now()
	// TODO (b/277656113): Collect GPU usage info for each DRM and save to ds.drms.

	ds.snapshotTime = append(ds.snapshotTime, time.Now().Sub(startTime))
	return nil
}

// Stop saves the collected GPU usage information to the perf values.
func (ds *GPUUsageDataSource) Stop(ctx context.Context, values *perf.Values) error {
	if ds.skip {
		return nil
	}
	testing.ContextLog(ctx, "Stop tracking GPU usage metrics")

	for range ds.drms {
		// TODO (b/277656113): Record GPU usage of each DRM into the perf values.
	}

	for pattern, log := range ds.logs {
		testing.ContextLogf(ctx, "GPU usage log pattern: %q; occurrence number: %d; example: %s", pattern, log.count, log.example)
	}
	if len(ds.snapshotTime) > 0 {
		var totalDuration float64
		for _, d := range ds.snapshotTime {
			totalDuration += float64(d / time.Millisecond)
		}
		// Print the snapshot average time so the performance of the GPU usage
		// collection can be evaluated.
		testing.ContextLogf(ctx, "GPU usage average time of doing snapshot: %f ms", totalDuration/float64(len(ds.snapshotTime)))

	}
	return nil
}
