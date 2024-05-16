// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package linuxperf

import (
	"bufio"
	"context"
	"io"
	"os"
	"regexp"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Frame stores one frame of a stack collected with a Linux Perf event sample.
type Frame struct {
	Address uint64
	Label   string
	Offset  int
	Module  string
}

// Event stores information on one Linux Perf event sample.
type Event struct {
	Comm      string
	Pid       int
	Tid       int
	CPU       int
	Timestamp float64
	Count     uint64
	Name      string

	// Stack is the call stack of the sample. The most recently called function is
	// at the beginning of the slice. May be nil.
	Stack []Frame
}

// Event example lines:
// "ThreadPoolForeg    6377/6382    [001] 610725.872521:    2800000 cpu-cycles: "
// "           chrome  1813/1813  [005]  1152.643253:    1091790 cpu-cycles:        7a01afd240 [unknown] (/usr/lib64/libmali.so.0.44.1)"
// "swapper     0 [000]  1536.496411:                     sched:sched_waking: comm=kworker/u17:1 pid=214 prio=100 target_cpu=000"
//
// ................................Comm......Pid.....Tid..........CPU.............Timestamp....Count.Name
var eventRE = regexp.MustCompile(`^(.*[^ ]) +(-?\d+)/(-?\d+) +\[0*(0|[1-9]\d*)\] +(\d+\.\d+): +(\d+) ([^ ]+): (?:.*)\n$`)

func parseEventLine(line string) (*Event, error) {
	match := eventRE.FindStringSubmatch(line)
	if match == nil {
		return nil, errors.New("not an event line")
	}
	comm := match[1]
	pid, err := strconv.Atoi(match[2])
	if err != nil {
		return nil, errors.Errorf("%q is not a valid pid", match[2])
	}
	tid, err := strconv.Atoi(match[3])
	if err != nil {
		return nil, errors.Errorf("%q is not a valid tid", match[3])
	}
	cpu, err := strconv.Atoi(match[4])
	if err != nil {
		return nil, errors.Errorf("%q is not a valid cpu", match[4])
	}
	timestamp, err := strconv.ParseFloat(match[5], 64)
	if err != nil {
		return nil, errors.Errorf("%q is not a valid timestamp", match[5])
	}
	count, err := strconv.ParseUint(match[6], 10, 64)
	if err != nil {
		return nil, errors.Errorf("%q is not a valid count", match[6])
	}
	name := match[7]

	return &Event{
		Comm:      comm,
		Pid:       pid,
		Tid:       tid,
		CPU:       cpu,
		Timestamp: timestamp,
		Count:     count,
		Name:      name,
	}, nil
}

// Frame example lines:
//
//	ffffffff98a9f00f inet_twsk_purge+0x1f ([kernel.kallsyms])
//	    59e12b64d8eb base::(anonymous namespace)::ThreadFunc(void*)+0xdb (/opt/google/chrome/chrome)
//	    5b870fd96769 [unknown] (/dev/shm/.com.google.Chrome.MdG6mi (deleted))
//
// ...................................Address...........Label...Offset........................Module
var frameRE = regexp.MustCompile(`^\s+([0-9A-Fa-f]+) (?:(.*)\+0x([0-9A-Fa-f]+)|\[unknown\]) \(([^)]*(?:\(deleted\))?)\)\n$`)

func parseFrameLine(line string) (*Frame, error) {
	match := frameRE.FindStringSubmatch(line)
	if match == nil {
		return nil, errors.New("not a frame line")
	}
	address, err := strconv.ParseUint(match[1], 16, 64)
	if err != nil {
		return nil, errors.Errorf("%q is not a valid address", match[1])
	}
	label := match[2]
	var offset uint64 = 0
	if match[3] != "" {
		offset, err = strconv.ParseUint(match[3], 16, 64)
		if err != nil {
			return nil, errors.Errorf("%q is not a valid offset", match[3])
		}
	}
	module := match[4]

	return &Frame{
		Address: address,
		Label:   label,
		Offset:  int(offset),
		Module:  module,
	}, nil
}

// DumpScript dumps the output of perf script to a file. Used for debugging
// eventCallbacks for Script. Linux perf script files can be very large, so
// be careful about using this in a real test.
func DumpScript(ctx context.Context, perfDataFile, outFilePath string) error {
	cmd := testexec.CommandContext(ctx, "perf", "script", "-F", "+pid", "-i", perfDataFile)
	outFile, err := os.Create(outFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to open perf script dump file %q", outFilePath)
	}
	cmd.Stdout = outFile
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to run perf script")
	}
	return nil
}

// Script runs perf script on a stopped perf profile, and calls each
// eventCallback for every event in the profile.
func Script(ctx context.Context, perfDataFile string, eventCallbacks ...func(*Event) error) error {
	cmd := testexec.CommandContext(ctx, "perf", "script", "-F", "+pid", "-i", perfDataFile)
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to run perf script")
	}

	outReader := bufio.NewReader(outPipe)

	var pendingEvent *Event
	for {
		line, err := outReader.ReadString('\n')
		if err == io.EOF {
			if pendingEvent != nil {
				for _, eventCallback := range eventCallbacks {
					if err := eventCallback(pendingEvent); err != nil {
						return err
					}
				}
			}
			return nil
		} else if err != nil {
			if stderrBytes, stderrErr := io.ReadAll(errPipe); stderrErr != nil {
				testing.ContextLogf(ctx, "Failed to read perf script output, stderr: %q", string(stderrBytes))
			}
			return errors.Wrap(err, "failed to read perf script output")
		}

		if pendingEvent != nil {
			// There is a pending event, consume stack lines if there are any.
			if line[0] == '\t' {
				// Stack lines begin with a tab.
				frame, err := parseFrameLine(line)
				if err != nil {
					return errors.Wrapf(err, "failed to parse frame %q", line)
				}
				pendingEvent.Stack = append(pendingEvent.Stack, *frame)
				continue
			} else if line == "\n" {
				// There is a blank line after the last stack line, if the event
				// has a stack.
				continue
			}
			// No more stack lines, pendingEvent is finished.
			for _, eventCallback := range eventCallbacks {
				if err := eventCallback(pendingEvent); err != nil {
					return err
				}
			}
			pendingEvent = nil
		}

		pendingEvent, err = parseEventLine(line)
		if err != nil {
			return errors.Wrapf(err, "failed to parse event %q", line)
		}
	}
}
