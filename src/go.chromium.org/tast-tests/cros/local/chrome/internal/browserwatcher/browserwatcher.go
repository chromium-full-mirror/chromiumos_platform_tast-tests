// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package browserwatcher provides a utility to monitor browser process for
// crashes.
package browserwatcher

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/local/chrome/internal/chromeproc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	checkBrowserInterval = 100 * time.Millisecond // interval to check browser process
)

// Process state description refers to https://man7.org/linux/man-pages/man5/proc_pid_stat.5.html.
const (
	// processStateIndex indicates the index of the process state (field 3) in /proc/<pid>/stat.
	processStateIndex = 2
	// processStartTimeIndex indicates indicates the index of the process start time (field 22) in /proc/<pid>/stat.
	processStartTimeIndex = 21
)

// Watcher watches the browser process to attempt to identify situations where Chrome is crashing.
type Watcher struct {
	proc       *process.Process // browser process
	browserErr error            // error that was detected, if any

	// procStartTime records the process start time from /proc/<PID>/stat.
	// Unlike createTime recorded in *process.Process, it is a relative
	// time since system boot.
	procStartTime uint64
}

// NewWatcher creates a new Watcher and starts it.
// Returns only when the chrome PID it's watching has been identified.
func NewWatcher(ctx context.Context, execPath string) (*Watcher, error) {
	// Wait for the browser process to start.
	var proc *process.Process
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		proc, err = chromeproc.Root(execPath)
		if err != nil {
			return errors.Wrap(err, "browser process not started yet")
		}
		return nil
	}, &testing.PollOptions{Interval: checkBrowserInterval}); err != nil {
		return nil, err
	}

	var startTime uint64
	var err error
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, startTime, err = getProcessInfo(proc.Pid)
		if err != nil {
			return errors.Wrap(err, "failed to parse start time from process status file")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to fetch information from process status file")
	}

	return &Watcher{proc: proc, procStartTime: startTime}, nil
}

// ReplaceErr returns the first error that was observed if any. Otherwise, it
// returns err as-is.
func (bw *Watcher) ReplaceErr(inErr error) error {
	if bw.browserErr == nil {
		// Check both running and err here to avoid edge cases
		// like recycling process ID.
		// See IsRunning implementation for details.
		running, err := procIsRunning(bw.proc.Pid, bw.procStartTime)
		if err == nil && running {
			// No error is found, so return the original error.
			return inErr
		}
		bw.browserErr = errors.Wrapf(err, "browser process %d exited; Chrome probably crashed", bw.proc.Pid)
	}
	// Some error was found, so return it instead of the given err.
	return bw.browserErr
}

// WaitExit polls until the *Watcher's target process is no longer running.
func (bw *Watcher) WaitExit(ctx context.Context) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if running, err := procIsRunning(bw.proc.Pid, bw.procStartTime); err != nil {
			return errors.Wrap(err, "failed to check browser process status")
		} else if running {
			return errors.New("chrome is still running")
		}
		return nil
	}, &testing.PollOptions{Interval: checkBrowserInterval})
}

func procIsRunning(pid int32, expectedStartTime uint64) (bool, error) {
	state, startTime, err := getProcessInfo(pid)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to get process info")
	}

	// Prevent the process ID from being reused.
	if startTime != expectedStartTime {
		return false, nil
	}

	// Z: Zombie, X/x: Dead.
	switch state {
	case "Z", "X", "x":
		return false, nil
	default:
		return true, nil
	}
}

func getProcessInfo(pid int32) (state string, startTime uint64, err error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%v/stat", pid))
	if err != nil {
		return "", 0, errors.Wrap(err, "failed to read process state file")
	}

	fields, err := fetchFieldsFromStatData(data)
	if err != nil {
		return "", 0, err
	}

	startTime, err = strconv.ParseUint(fields[processStartTimeIndex], 10, 64)
	if err != nil {
		return "", 0, errors.Wrap(err, "failed to parse start time from process state file")
	}

	return fields[processStateIndex], startTime, nil
}

func fetchFieldsFromStatData(data []byte) ([]string, error) {
	statStr := string(data)

	// Find the boundaries of the process name.
	leftIdx := strings.Index(statStr, "(")
	rightIdx := strings.LastIndex(statStr, ")")
	if leftIdx == -1 || rightIdx == -1 || leftIdx >= rightIdx {
		return nil, errors.New(`invalid /proc stat format, missing or invalid parentheses`)
	}

	// Parse PID, process name, and the remaining fields separately.
	pid := strings.Fields(statStr[:leftIdx])
	if len(pid) != 1 {
		return nil, errors.New(`invalid /proc stat format, invalid PID`)
	}

	processName := statStr[leftIdx+1 : rightIdx]
	stat := strings.Fields(statStr[rightIdx+1:])
	fields := append(append(pid, processName), stat...)

	if len(fields) < processStartTimeIndex+1 {
		return nil, errors.New(`invalid /proc stat format, insufficient fields length for check start time`)
	}

	return fields, nil
}
