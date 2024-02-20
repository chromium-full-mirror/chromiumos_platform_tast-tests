// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mlbenchmark contains helper functions to easily create
// new ML based benchmarks.
package mlbenchmark

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/async"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// DataDirectory is the location to unpack any associated data files into.
	DataDirectory = "/usr/local/mlbenchmark/data"
)

// UnpackData will untar the file specified by `dataPath` into `DataDirectory`.
func UnpackData(ctx context.Context, dataPath string) error {
	testing.ContextLogf(ctx, "unpacking %s into %s", dataPath, DataDirectory)
	tarCmd := testexec.CommandContext(ctx, "tar", "-xvf", dataPath, "-C", DataDirectory)
	if err := tarCmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to untar test artifacts")
	}

	return nil
}

// DataPath returns the absolute path for a data file that was extracted with `UnpackData`.
func DataPath(f string) string {
	return filepath.Join(DataDirectory, f)
}

// BuildCommand will construct a `Cmd` object using `execName` as the binary and `args` as the
// set of arguments to pass to the executable.
func BuildCommand(ctx context.Context, execName string, args map[string]string) *testexec.Cmd {
	var argsAsStr = []string{}
	for key, value := range args {
		argsAsStr = append(argsAsStr, fmt.Sprintf("%s=%s", key, value))
	}
	return testexec.CommandContext(ctx, execName, argsAsStr...)
}

// ParseNumeric will parse a float64 from an input string. Supports scientific notation.
func ParseNumeric(input string) (float64, error) {
	flt, _, err := big.ParseFloat(input, 10, 0, big.ToNearestEven)
	if err != nil {
		return 0, errors.Wrapf(err, "couldn't parse input: %s", input)
	}
	f, _ := flt.Float64()
	return f, nil
}

// PeakMemoryWatcher is a utility to poll a process's memory and measure peak usage.
type PeakMemoryWatcher struct {
	PeakGpuMemory float64
	PeakRssMemory float64
	PeakMemory    float64

	interval     time.Duration // Time to wait between polling
	pid          int
	isRunning    bool
	runnerStatus chan error
}

// NewPeakMemoryWatcher will create a new PeakMemoryWatcher object to poll a process's
// memory and measure peak usage.
func NewPeakMemoryWatcher(pid int) *PeakMemoryWatcher {
	return &PeakMemoryWatcher{
		interval: time.Millisecond * 50,
		pid:      pid,
	}
}

// Start will begin running the watcher.
func (r *PeakMemoryWatcher) Start(ctx context.Context) error {
	if r.isRunning {
		return errors.New("watcher already running")
	}

	r.isRunning = true
	r.runnerStatus = make(chan error, 1)

	async.Run(ctx, func(ctx context.Context) {
		for r.isRunning {
			// Get RSS size
			statusFile := "/proc/" + strconv.Itoa(r.pid) + "/status"
			currentRss, currentMaxGpu := 0.0, 0.0
			f, err := os.Open(statusFile)
			if err != nil {
				testing.ContextLogf(ctx, "Couldn't open process status file: %+v", err)
			} else {
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					re := regexp.MustCompile(`\bVmRSS:\s*(\d+)`)
					matches := re.FindStringSubmatch(scanner.Text())
					if len(matches) == 2 {
						currentRss, err = ParseNumeric(matches[1])
						currentRss = math.Floor(currentRss / 1024) // Get MB
						if err != nil {
							r.runnerStatus <- errors.Wrap(err, "couldn't parse VmRSS")
							return
						}
						r.PeakRssMemory = math.Max(r.PeakRssMemory, currentRss)
					}
				}
			}

			fdinfoDir := "/proc/" + strconv.Itoa(r.pid) + "/fdinfo/"
			files, _ := ioutil.ReadDir(fdinfoDir)
			for _, file := range files {
				f, err := os.Open(fdinfoDir + file.Name())
				if err != nil {
					testing.ContextLogf(ctx, "Couldn't open process fdinfo file: %+v", err)
					continue
				}
				defer f.Close()

				total, shared := 0.0, 0.0
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					re := regexp.MustCompile(`\bdrm-(total|shared)-memory:\s*(\d+)`)
					matches := re.FindStringSubmatch(scanner.Text())
					if len(matches) == 3 {
						memory, err := ParseNumeric(matches[2])
						if err != nil {
							r.runnerStatus <- errors.Wrap(err, "couldn't parse drm memory")
							return
						}
						if matches[1] == "total" {
							total = memory
						}
						if matches[1] == "shared" {
							shared = memory
						}
					}
				}
				currentMaxGpu = math.Max(currentMaxGpu, total-shared)
			}

			r.PeakGpuMemory = math.Max(r.PeakGpuMemory, currentMaxGpu)
			r.PeakMemory = math.Max(r.PeakMemory, currentMaxGpu+currentRss)

			// GoBigSleepLint: Sleep for user specified time.
			if err := testing.Sleep(ctx, r.interval); err != nil {
				r.runnerStatus <- errors.Wrap(err, "failed to wait between polling memory")
				return
			}
		}
		r.runnerStatus <- nil

	}, "PeakMemoryWatcher")

	return nil
}

// Stop will finish the background memory poll.
func (r *PeakMemoryWatcher) Stop() error {
	if !r.isRunning {
		return errors.New("watcher isn't running")
	}

	r.isRunning = false

	var err error
	select {
	case err = <-r.runnerStatus:
	case <-time.After(time.Minute):
		panic("PeakMemoryWatcher.Stop timed out")
	}
	r.runnerStatus = nil
	return err
}
