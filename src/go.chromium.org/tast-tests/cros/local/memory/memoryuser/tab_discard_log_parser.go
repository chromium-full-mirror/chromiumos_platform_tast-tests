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

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/errors"
)

// TabDiscardInfo tracks information for one tab discard action.
type TabDiscardInfo struct {
	// Time is when the tab was discarded.
	Time time.Time
	// ID is the ID of the discarded tab.
	ID int
	// Priority is the priority of the discarded tab.
	Priority string
}

// VmmmsPriority converts the TabDiscardInfo.Priority to a VmmmsPriority.
func (i TabDiscardInfo) VmmmsPriority() VmmmsPriority {
	switch i.Priority {
	case "BACKGROUND":
		return VmmmsCachedTabPriority
	case "PROTECTED_BACKGROUND":
		return VmmmsPerceptibleTabPriority
	default:
		return -1
	}
}

// 2023-01-18T03:37:17.807226Z ERROR chrome[23832:23832]: [device_event_log_impl.cc(221)] [12:37:17.807] Memory: tab_manager_delegate_chromeos.cc:552 tab (id: 1, pid: 24754), process_type BACKGROUND
var discardCandidateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z ERROR chrome\[\d+:\d+\]: \[[^\]]*\] \[\d{2}:\d{2}:\d{2}\.\d{3}\] Memory: tab_manager_delegate_chromeos.cc:\d+ tab \(id: (\d+), pid: \d+\), process_type (BACKGROUND|PROTECTED_BACKGROUND)`)

// 2023-01-18T03:37:18.316273Z ERROR chrome[23832:23832]: [device_event_log_impl.cc(221)] [12:37:18.316] Memory: tab_manager_delegate_chromeos.cc:617 Killed tab (id: 1), estimated 306836 KB freed
var discardRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z) ERROR chrome\[\d+:\d+\]: \[[^\]]*\] \[\d{2}:\d{2}:\d{2}\.\d{3}\] Memory: tab_manager_delegate_chromeos.cc:\d+ Killed tab \(id: (\d+)\)`)

// ParseTabDiscards reads all the tab_manager_delegate Killed tab lines from the
// Chrome logs between the given time stamps and returns a list of
// TabDiscardInfo in chronological order.
func ParseTabDiscards(ctx context.Context, cr *chrome.Chrome, start, stop time.Time) ([]*TabDiscardInfo, error) {
	fileName := cr.LogFilename()
	reader, err := syslog.NewLineReader(ctx, fileName, true, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open %q to parse tab discards", fileName)
	}

	tabPri := make(map[int]string) // Tab ID -> priority string.
	var log []*TabDiscardInfo
	for {
		line, err := reader.ReadLine()
		if err == io.EOF {
			return log, nil
		} else if err != nil {
			return nil, errors.Wrapf(err, "failed to read %q looking for tab discards", fileName)
		}

		if m := discardCandidateRE.FindStringSubmatch(line); m != nil {
			// Line is from a list of tab discard candidates, save the priority for later.
			id, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ID for tab discard candidate")
			}
			tabPri[id] = m[2]
		} else if m := discardRE.FindStringSubmatch(line); m != nil {
			// Line is a tab discard record.
			t, err := time.ParseInLocation(time.RFC3339Nano, m[1], time.UTC)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse time stamp of tab discard")
			}
			if t.After(stop) {
				// The discard is after the stop time, we are done.
				return log, nil
			}
			id, err := strconv.Atoi(m[2])
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ID for discarded tab")
			}
			pri, priExists := tabPri[id]
			if !priExists {
				return nil, errors.Errorf("discarded tab %d was never a candidate", id)
			}

			log = append(log, &TabDiscardInfo{
				Time:     t,
				ID:       id,
				Priority: pri,
			})
		}
	}
}

// FirstTabDiscardOfPriority returns the first tab discard of the provided
// priority or nil if none exist.
func FirstTabDiscardOfPriority(log []*TabDiscardInfo, priority VmmmsPriority) *TabDiscardInfo {
	for _, info := range log {
		if info.VmmmsPriority() == priority {
			return info
		}
	}
	return nil
}
