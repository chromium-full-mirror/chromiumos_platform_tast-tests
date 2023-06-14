// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"context"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/hps/hpsutil"
	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	memoryInfo       = "/proc/self/status"
	rssName          = "VmRSS"
	swapName         = "VmSwap"
	fieldFilter      = `:\s+(\d+) kB`
	crosCameraFilter = `root(\s+)(\d+)(.+)cros_camera_service`

	// DefaultTimeInterval defines the default time interval duration for power metric collection.
	DefaultTimeInterval = 5 * time.Second
	// DefaultTestDuration defines the default test duration of 5 minutes in seconds.
	DefaultTestDuration = 60 * 5
)

// PeakMemoryResult returns the peak memory or an error.
type PeakMemoryResult struct {
	Value int
	Err   error
}

// GetPercentileData returns a map of percentiles and their corresponding data for the given array.
func GetPercentileData(ctx context.Context, data []float64) (map[int]float64, error) {
	percentile := []int{50, 90, 95, 99}

	sort.Sort(sort.Reverse(sort.Float64Slice(data)))

	m := make(map[int]float64)
	for _, p := range percentile {
		value, err := hpsutil.PercentileForSortedData(data, p)
		if err != nil {
			return m, errors.Wrapf(err, "error calculating %q percentile", p)
		}
		m[p] = value
	}
	return m, nil
}

func getCameraServicePID(ctx context.Context) (int, error) {
	// Find cros_camera_service pid.
	pids, err := testexec.CommandContext(ctx, "ps", "-ef").CombinedOutput()
	if err != nil {
		return -1, errors.Wrap(err, "failed to execute testexec command")
	}
	cregex := regexp.MustCompile(crosCameraFilter)
	result := cregex.FindStringSubmatch(string(pids))
	if len(result) < 3 {
		return -1, errors.New("no pid for cros_camera_service found")
	}
	return strconv.Atoi(result[2])
}

func readProcFSMemoryField(ctx context.Context, fieldName string) (int, error) {
	pid, err := getCameraServicePID(ctx)
	if err != nil {
		return -1, errors.Wrap(err, "getCameraServicePID failed")
	}
	cameraProcPath := strings.Replace(memoryInfo, "self", strconv.Itoa(pid), -1)
	fileContent, err := os.ReadFile(cameraProcPath)
	if err != nil {
		return -1, errors.Wrap(err, "failed to read camera proc path")
	}

	fregex := regexp.MustCompile(fieldName + fieldFilter)
	result := fregex.FindStringSubmatch(string(fileContent))

	if len(result) < 2 {
		return -1, errors.New("no memory usage value found")
	}
	value, err := strconv.Atoi(result[1])
	if err != nil {
		return -1, errors.Wrapf(err, "conversion failed: %q", result[1])
	}
	return value, nil
}

// GetSwapAndRSSBytes gets the sum of RSS and Swap field from system.
func GetSwapAndRSSBytes(ctx context.Context) (int, error) {
	rssVal, err := readProcFSMemoryField(ctx, "VmRSS")
	if err != nil {
		return -1, errors.Wrap(err, "readProcFSMemoryField VmRSS failed")
	}

	swapVal, err := readProcFSMemoryField(ctx, "VmSwap")
	if err != nil {
		return -1, errors.Wrap(err, "readProcFSMemoryField VmSwap failed")
	}
	return (rssVal + swapVal) * 1024, nil
}

// GetMaxMemoryUsage gets the memory peak usage during the run time.
func GetMaxMemoryUsage(ctx context.Context, result chan PeakMemoryResult, seconds int) {
	testing.ContextLog(ctx, "Start recording memory usage for ", seconds, " seconds")
	var maxMemUsage = 0
	for start := time.Now(); time.Since(start) < time.Duration(seconds)*time.Second; {
		// GoBigSleepLint: Get memory usage every 5 seconds for 1 min.
		testing.Sleep(ctx, 5*time.Second)
		currentMemUsage, err := GetSwapAndRSSBytes(ctx)
		if err != nil {
			result <- PeakMemoryResult{Value: -1, Err: err}
			return
		}
		if maxMemUsage < currentMemUsage {
			maxMemUsage = currentMemUsage
		}
	}
	result <- PeakMemoryResult{Value: maxMemUsage, Err: nil}
}
