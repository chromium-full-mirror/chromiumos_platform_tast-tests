// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package profiler

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tklauser/go-sysconf"

	"go.chromium.org/tast/core/errors"
)

type statSnapshot struct {
	wall time.Time // time the snapshot was taken
	user int64     // user time, measured in ticks
	sys  int64     // sys time, measured in ticks
}

// takeStatSnapshot takes a snapshot on /proc/[pid]/stat.
func takeStatSnapshot(pid int) (*statSnapshot, error) {
	s := &statSnapshot{
		wall: time.Now(),
	}

	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return nil, err
	}

	s.user, s.sys, err = parseProcStat(string(b))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parseProcStat(%q)", string(b))
	}
	return s, nil
}

// parseProcStat parses a string read from /proc/pid/stat.
//
// The stat looks like this:
//
//	496189 (comm) R 448300 496189 448300 34826 ...
//
// where comm is the filename of the executable.
func parseProcStat(stat string) (user, sys int64, err error) {
	const (
		// /proc/[pid]/stat fields
		// https://man7.org/linux/man-pages/man5/proc.5.html
		commIdx  = 2
		utimeIdx = 14
		stimeIdx = 15
	)

	// The prog name may include spaces and parentheses.
	// Find the last parenthesis and parse from there.
	i := strings.LastIndexByte(stat, ')')
	if i == -1 {
		return 0, 0, errors.New("')' not found")
	}
	stat = stat[i:]

	parts := strings.Split(stat, " ")
	// Now the first item in parts is ")".
	if len(parts) <= stimeIdx-commIdx {
		return 0, 0, errors.New("too few fields")
	}

	user, err = strconv.ParseInt(parts[utimeIdx-commIdx], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	sys, err = strconv.ParseInt(parts[stimeIdx-commIdx], 10, 64)
	return user, sys, err
}

// procStat implements profiler.instance.
type procStat struct {
	// Pointer to the output struct.
	out *ProcStatOutput

	// Process ID to profile.
	pid int

	// The Snapshot taken on the start of the profile.
	startSnapshot *statSnapshot
	// Cached value of sysconf(_SC_CLK_TCK).
	ticksPerSeconds int64
}

var _ instance = &procStat{}

// ProcStatOutput stores the output of ProfStat.
type ProcStatOutput struct {
	WallTime time.Duration
	UserTime time.Duration
	SysTime  time.Duration
}

// CPUUtilization returns the CPU Utilization during the sampled period,
// calculated as (user+sys)/wall.
//
// Also known as %CPU displayed by top(1).
func (o *ProcStatOutput) CPUUtilization() float64 {
	return float64(o.UserTime+o.SysTime) / float64(o.WallTime)
}

// ProcStat returns a Profiler that measures the wall/user/sys time of
// a process, specified by its pid.
func ProcStat(out *ProcStatOutput, pid int) Profiler {
	return func(ctx context.Context, outDir string) (instance, error) {
		return newProcStat(ctx, out, pid)
	}
}

func newProcStat(ctx context.Context, out *ProcStatOutput, pid int) (instance, error) {
	startSnapshot, err := takeStatSnapshot(pid)
	if err != nil {
		return nil, errors.Wrap(err, "cannot take initial snapshot")
	}

	tps, err := sysconf.Sysconf(sysconf.SC_CLK_TCK)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get ticks per seconds on system")
	}

	return &procStat{
		out:             out,
		pid:             pid,
		startSnapshot:   startSnapshot,
		ticksPerSeconds: tps,
	}, nil
}

func (s *procStat) end(ctx context.Context) error {
	endSnapshot, err := takeStatSnapshot(s.pid)
	if err != nil {
		return errors.Wrap(err, "cannot take end snapshot")
	}

	s.out.WallTime = endSnapshot.wall.Sub(s.startSnapshot.wall)
	s.out.UserTime = ticksToDuration(endSnapshot.user-s.startSnapshot.user, s.ticksPerSeconds)
	s.out.SysTime = ticksToDuration(endSnapshot.sys-s.startSnapshot.sys, s.ticksPerSeconds)

	return nil
}

func ticksToDuration(ticks, ticksPerSecond int64) time.Duration {
	return time.Duration(ticks) * time.Second / time.Duration(ticksPerSecond)
}
