// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"context"
	"io"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/errors"
)

// TabDiscardInfo tracks information for one tab discard action.
type TabDiscardInfo struct {
	// Time is when the tab was discarded.
	Time time.Time
	// Priority is the priority of the discarded tab.
	Priority VmmmsPriority
}

// 2024-09-30T06:58:58.176402Z WARNING chrome[10008:10026]: [page_discarding_helper.cc(213)] Queueing discard attempt, type=kTab, flags=[ protected visible ] to save 81920 KiB
var discardRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z) .* Queueing discard attempt, type=kTab, flags=\[(.*)\] to save [\d]+ KiB`)

// ParseTabDiscards reads all the tab_manager_delegate Killed tab lines from the
// Chrome logs between the given time stamps and returns a list of
// TabDiscardInfo in chronological order.
func ParseTabDiscards(ctx context.Context, cr *chrome.Chrome, start, stop time.Time) ([]*TabDiscardInfo, error) {
	fileName := cr.LogFilename()
	reader, err := syslog.NewLineReader(ctx, fileName, true, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open %q to parse tab discards", fileName)
	}

	var log []*TabDiscardInfo
	for {
		line, err := reader.ReadLine()
		if err == io.EOF {
			return log, nil
		} else if err != nil {
			return nil, errors.Wrapf(err, "failed to read %q looking for tab discards", fileName)
		}

		if m := discardRE.FindStringSubmatch(line); m != nil {
			// Line is a tab discard record.
			t, err := time.ParseInLocation(time.RFC3339Nano, m[1], time.UTC)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse time stamp of tab discard")
			}
			if t.After(stop) {
				// The discard is after the stop time, we are done.
				return log, nil
			}

			pri := VmmmsCachedTabPriority

			if strings.Contains(m[2], "protected") {
				pri = VmmmsPerceptibleTabPriority
			}

			log = append(log, &TabDiscardInfo{
				Time:     t,
				Priority: pri,
			})
		}
	}
}

// FirstTabDiscardOfPriority returns the first tab discard of the provided
// priority or nil if none exist.
func FirstTabDiscardOfPriority(log []*TabDiscardInfo, priority VmmmsPriority) *TabDiscardInfo {
	for _, info := range log {
		if info.Priority == priority {
			return info
		}
	}
	return nil
}
