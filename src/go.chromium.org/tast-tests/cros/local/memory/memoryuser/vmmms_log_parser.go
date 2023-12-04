// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"context"
	"io"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VmmmsPriority is the priority of resize requests sent to the Virtual Machine
// Memory Management Service.
type VmmmsPriority int

const (
	// VmmmsCachedAppPriority is the priority of a cached background Android app.
	VmmmsCachedAppPriority VmmmsPriority = iota

	// VmmmsCachedTabPriority is the priority of a background tab.
	VmmmsCachedTabPriority

	// VmmmsPerceptibleAppPriority is the priority of a perceptible Android app.
	VmmmsPerceptibleAppPriority

	// VmmmsPerceptibleTabPriority is the priority of a perceptible tab.
	VmmmsPerceptibleTabPriority

	// VmmmsFocusedAppPriority is the priority of the focused Android app.
	VmmmsFocusedAppPriority
)

// VmmmsKillPriorityToString converts a VmmmsPriority to a human readable
// string.
func VmmmsKillPriorityToString(priority VmmmsPriority) string {
	switch priority {
	case VmmmsFocusedAppPriority:
		return "RESIZE_PRIORITY_FOCUSED_APP"
	case VmmmsPerceptibleTabPriority:
		return "RESIZE_PRIORITY_PERCEPTIBLE_TAB"
	case VmmmsPerceptibleAppPriority:
		return "RESIZE_PRIORITY_PERCEPTIBLE_APP"
	case VmmmsCachedTabPriority:
		return "RESIZE_PRIORITY_CACHED_TAB"
	case VmmmsCachedAppPriority:
		return "RESIZE_PRIORITY_CACHED_APP"
	default:
		return "UNKNOWN"
	}
}

// ParseVmmmsKillPriority parses a VmmmsPriority as represented in concierge
// logs.
func ParseVmmmsKillPriority(s string) (VmmmsPriority, error) {
	switch s {
	case "RESIZE_PRIORITY_FOCUSED_APP":
		return VmmmsFocusedAppPriority, nil
	case "RESIZE_PRIORITY_PERCEPTIBLE_TAB":
		return VmmmsPerceptibleTabPriority, nil
	case "RESIZE_PRIORITY_PERCEPTIBLE_APP":
		return VmmmsPerceptibleAppPriority, nil
	case "RESIZE_PRIORITY_CACHED_TAB":
		return VmmmsCachedTabPriority, nil
	case "RESIZE_PRIORITY_CACHED_APP":
		return VmmmsCachedAppPriority, nil
	default:
		return -1, errors.Errorf("unexpected VMMMS KillTrace priority %q", s)
	}
}

// VmmmsKillInfo describes a kill observed by the VM Memory Management Service.
type VmmmsKillInfo struct {
	Time     time.Time
	Cid      int64
	Priority VmmmsPriority
	SizeMiB  int64
}

// 2023-10-31T07:32:23.450025Z INFO vm_concierge[21465]: KillTrace:[35,RESIZE_PRIORITY_CACHED_APP,53MB]
var killTraceRE = regexp.MustCompile(`^(?P<timestamp>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}(?:Z|[+-]\d{2}:\d{2})) INFO vm_concierge\[\d+\]: KillTrace:\[(?P<cid>\d+),(?P<priority>[A-Z_]+),(?P<sizeMB>\d+)MB\]`)

// ParseVmmmsKills reads all KillTrace lines from /var/log/messages between the
// given time stamps and returns a list of VmmmsKillInfo in chronological order.
func ParseVmmmsKills(ctx context.Context, start, stop time.Time) ([]*VmmmsKillInfo, error) {
	var log []*VmmmsKillInfo
	reader, err := syslog.NewLineReader(ctx, "/var/log/messages", true, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open /var/log/messages to parse VMMMS KillTrace lines")
	}

	for {
		line, err := reader.ReadLine()
		if err == io.EOF {
			return log, nil
		} else if err != nil {
			return nil, errors.Wrap(err, "failed to read /var/log/messages looking for KillTrace lines")
		}

		m := killTraceRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		t, err := time.Parse(time.RFC3339Nano, m[1])
		if err != nil {
			return nil, errors.Errorf("invalid timestamp for VMMMS kill log line %q", m[1])
		}
		if t.Before(start) {
			continue
		}
		if t.After(stop) {
			return log, nil
		}

		cid, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse VM CID %q", m[2])
		}

		priority, err := ParseVmmmsKillPriority(m[3])
		if err != nil {
			return nil, err
		}

		size, err := strconv.ParseInt(m[4], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse VMMMS kill size %q", m[4])
		}
		log = append(log, &VmmmsKillInfo{
			Time:     t,
			Cid:      cid,
			Priority: priority,
			SizeMiB:  size,
		})
	}
}

// FirstVmmmsKillOfPriority returns the first VmmmsKillInfo with a given
// priority, or nil if none exists.
func FirstVmmmsKillOfPriority(log []*VmmmsKillInfo, priority VmmmsPriority) *VmmmsKillInfo {
	for _, info := range log {
		if info.Priority == priority {
			return info
		}
	}
	return nil
}

// AllVmmmsPrioritiesLogged returns true if all priorities are observed in a
// log.
func AllVmmmsPrioritiesLogged(log []*VmmmsKillInfo) bool {
	priorities := make(map[VmmmsPriority]bool)
	for _, info := range log {
		priorities[info.Priority] = true
	}
	return priorities[VmmmsFocusedAppPriority] && priorities[VmmmsPerceptibleTabPriority] && priorities[VmmmsPerceptibleAppPriority] && priorities[VmmmsCachedTabPriority] && priorities[VmmmsCachedAppPriority]
}

// DumpVmmmsKillLog prints out a log of VmmmsKillInfo.
func DumpVmmmsKillLog(ctx context.Context, log []*VmmmsKillInfo) {
	for _, info := range log {
		testing.ContextLogf(ctx, "%s KillTrace %d, %s, %d MiB", info.Time.String(), info.Cid, VmmmsKillPriorityToString(info.Priority), info.SizeMiB)
	}
}
