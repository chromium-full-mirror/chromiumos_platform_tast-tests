// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// lmkdKillRE matches LMKD kill logcat lines, e.g.:
// 02-15 14:25:06.702   106   106 I lowmemorykiller: Kill 'org.chromium.arc.testapp.lifecycle00' (3579), uid 10071, oom_score_adj 103 to free 69128kB rss, 0kb swap
var lmkdKillRE = regexp.MustCompile(`^[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3} +[0-9]+ +[0-9]+ I lowmemorykiller: Kill '(?P<package>[^']+)' \((?P<pid>[0-9]+)\), uid (?P<uid>[0-9]+), oom_(?:score_)?adj (?P<oomscore>[0-9]+) `)

// LmkdKillInfo tracks information for one LMKD app kill.
type LmkdKillInfo struct {
	Time        time.Time
	Package     string
	PID         int
	UID         int
	OomScoreAdj int
}

func parseLmkdKillInfo(logcatLine string) *LmkdKillInfo {
	t, err := adb.ParseLogcatTimestamp(logcatLine)
	if err != nil {
		return nil
	}

	m := lmkdKillRE.FindStringSubmatch(logcatLine)
	if m == nil {
		return nil
	}

	pid, err := strconv.Atoi(m[2])
	if err != nil {
		return nil
	}

	uid, err := strconv.Atoi(m[3])
	if err != nil {
		return nil
	}

	oomScoreAdj, err := strconv.Atoi(m[4])
	if err != nil {
		return nil
	}

	return &LmkdKillInfo{
		Time:        t,
		Package:     m[1],
		PID:         pid,
		UID:         uid,
		OomScoreAdj: oomScoreAdj,
	}
}

// VmmmsPriority provides the VmmmsPriority for an LMKD kill.
func (i LmkdKillInfo) VmmmsPriority() VmmmsPriority {
	switch {
	case i.OomScoreAdj > 1000:
		return -1
	case i.OomScoreAdj >= 900: // ProcessList.CACHED_APP_MIN_ADJ
		return VmmmsCachedAppPriority
	case i.OomScoreAdj > 0: // ProcessList.FOREGROUND_APP_ADJ
		return VmmmsPerceptibleAppPriority
	case i.OomScoreAdj == 0:
		return VmmmsFocusedAppPriority
	default:
		return -1
	}
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

// ParseLmkdKills parses all LMKD kills in logcat. Returns a slice of LmkdKillInfo.
func ParseLmkdKills(ctx context.Context, a *arc.ARC, since adb.LogcatTimestampLong) ([]*LmkdKillInfo, error) {
	// We write a unique string to logcat so that when we observe it we know that
	// we are caught up with all lines logged before this call.
	doneString := fmt.Sprintf("ParseLmkdKills::Done::[%16x]", rand.Uint64())
	if err := a.Command(ctx, "log", doneString).Run(); err != nil {
		return nil, errors.Wrapf(err, "failed to log %q to synchronize logcat parsing", doneString)
	}

	var result []*LmkdKillInfo
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		result = nil
		if err := a.ScanLogcatSince(func(line string) bool {
			if strings.Contains(line, doneString) {
				return true
			}
			killInfo := parseLmkdKillInfo(line)
			if killInfo != nil {
				result = append(result, killInfo)
			}
			return false
		}, since); err != nil {
			testing.ContextLog(ctx, "Failed to scan logcat for LMKD kills: ", err)
			return err
		}
		return nil
	}, &testing.PollOptions{Interval: 10 * time.Second, Timeout: time.Minute}); err != nil {
		return nil, errors.Wrap(err, "failed to scan logcat for LMKD kills")
	}
	return result, nil
}
