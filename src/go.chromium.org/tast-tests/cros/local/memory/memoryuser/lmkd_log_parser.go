// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
)

var lmkdKillRE = regexp.MustCompile(`^[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3} +[0-9]+ +[0-9]+ I lowmemorykiller: Kill '(?P<package>[^']+)' \((?P<pid>[0-9]+)\), uid (?P<uid>[0-9]+), oom_(?:score_)?adj (?P<oomscore>[0-9]+) `)

// LmkdKillInfo tracks information for one LMKD app kill.
type LmkdKillInfo struct {
	Time        time.Time
	Package     string
	PID         int
	UID         int
	OomScoreAdj int
}

// VmmmsPriority provides the VmmmsPriority for an LMKD kill.
func (i LmkdKillInfo) VmmmsPriority() VmmmsPriority {
	if i.OomScoreAdj > 1000 {
		return -1
	} else if i.OomScoreAdj >= 900 { // ProcessList.CACHED_APP_MIN_ADJ
		return VmmmsCachedAppPriority
	} else if i.OomScoreAdj > 0 { // ProcessList.FOREGROUND_APP_ADJ
		return VmmmsPerceptibleAppPriority
	} else if i.OomScoreAdj == 0 {
		return VmmmsFocusedAppPriority
	} else {
		return -1
	}
}

// ParseLmkdKills reads all the "lowmemorykiller: Kill" lines from logcat
// between the provided timestamps and returns a list of LmkdKillInfo in
// chronological order.
func ParseLmkdKills(ctx context.Context, a *arc.ARC, start adb.LogcatTimestampLong, stop time.Time) ([]*LmkdKillInfo, error) {
	var log []*LmkdKillInfo
	var parseError error
	if err := a.WaitForLogcatSince(ctx, func(line string) bool {
		t, err := adb.ParseLogcatTimestamp(line)
		if err != nil {
			// Some lines don't have timestamps, so ignore failures
			return false
		}
		if t.After(stop) {
			return true
		}

		m := lmkdKillRE.FindStringSubmatch(line)
		if m == nil {
			return false
		}

		pid, err := strconv.Atoi(m[2])
		if err != nil {
			parseError = err
			return true
		}

		uid, err := strconv.Atoi(m[3])
		if err != nil {
			parseError = err
			return true
		}

		oomScoreAdj, err := strconv.Atoi(m[4])
		if err != nil {
			parseError = err
			return true
		}

		log = append(log, &LmkdKillInfo{
			Time:        t,
			Package:     m[1],
			PID:         pid,
			UID:         uid,
			OomScoreAdj: oomScoreAdj,
		})
		return false
	}, start); err != nil {
		return nil, errors.Wrap(err, "failed to scan logcat for LMKD kills")
	}
	if parseError != nil {
		return nil, errors.Wrap(parseError, "failed to parse logcat line while searching for LMKD logs")
	}
	return log, nil
}

// FirstLmkdKillOfPriority returns the first LmkdKillInfo of a given priority or
// nil if none exist.
func FirstLmkdKillOfPriority(log []*LmkdKillInfo, priority VmmmsPriority) *LmkdKillInfo {
	for _, info := range log {
		if info.VmmmsPriority() == priority {
			return info
		}
	}
	return nil
}
