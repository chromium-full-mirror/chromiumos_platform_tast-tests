// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"bufio"
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TabDiscardInfo describes a tab discard event in Chrome.
type TabDiscardInfo struct {
	Time time.Time
}

// TabDiscardObserver monitors Chrome logs to record tab discards.
type TabDiscardObserver struct {
	cancel context.CancelFunc

	// tab_manager_delegate_chromeos logs tab priority and actual discard events
	// in separate lines. Tabs are identified by a stable id, which this map keys
	// on to record and resolve tab priorities.
	tabPri map[int]string

	// The first discard event of the given priority that occurred after this
	// observer was constructed or reset.
	Background          *TabDiscardInfo
	ProtectedBackground *TabDiscardInfo

	// The most recent error encountered by this observer.
	Error error
}

// Close causes this observer to stop monitoring for new kill events.
func (o *TabDiscardObserver) Close() {
	o.cancel()
}

// AllPrioritiesObserved returns true if all priorities of tabs have been
// discarded.
func (o *TabDiscardObserver) AllPrioritiesObserved() bool {
	return o.Background != nil && o.ProtectedBackground != nil
}

// Reset clears any recorded kill events to allow new ones to be recorded.
func (o *TabDiscardObserver) Reset() {
	o.Background = nil
	o.ProtectedBackground = nil
}

// 2023-01-18T03:37:17.807226Z ERROR chrome[23832:23832]: [device_event_log_impl.cc(221)] [12:37:17.807] Memory: tab_manager_delegate_chromeos.cc:552 tab (id: 1, pid: 24754), process_type BACKGROUND
var discardCandidateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z ERROR chrome\[\d+:\d+\]: \[[^\]]*\] \[\d{2}:\d{2}:\d{2}\.\d{3}\] Memory: tab_manager_delegate_chromeos.cc:\d+ tab \(id: (\d+), pid: \d+\), process_type (BACKGROUND|PROTECTED_BACKGROUND)`)

// 2023-01-18T03:37:18.316273Z ERROR chrome[23832:23832]: [device_event_log_impl.cc(221)] [12:37:18.316] Memory: tab_manager_delegate_chromeos.cc:617 Killed tab (id: 1), estimated 306836 KB freed
var discardRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z) ERROR chrome\[\d+:\d+\]: \[[^\]]*\] \[\d{2}:\d{2}:\d{2}\.\d{3}\] Memory: tab_manager_delegate_chromeos.cc:\d+ Killed tab \(id: (\d+)\)`)

func (o *TabDiscardObserver) observeLine(ctx context.Context, line string) {
	var recordLocation **TabDiscardInfo
	var timeString string
	if groups := discardCandidateRE.FindStringSubmatch(line); groups != nil {
		id, err := strconv.Atoi(groups[1])
		if err != nil {
			testing.ContextLog(ctx, "Failed to parse ID for tab discard candidate: ", err)
			return
		}
		o.tabPri[id] = groups[2]
		return
	} else if groups := discardRE.FindStringSubmatch(line); groups != nil {
		timeString = groups[1]

		id, err := strconv.Atoi(groups[2])
		if err != nil {
			testing.ContextLog(ctx, "Failed to parse ID for discarded tab: ", err)
			return
		}
		pri, priExists := o.tabPri[id]
		if !priExists {
			testing.ContextLog(ctx, "Discarded tab was never a candidate")
			return
		}
		switch pri {
		case "BACKGROUND":
			recordLocation = &o.Background
		case "PROTECTED_BACKGROUND":
			recordLocation = &o.ProtectedBackground
		default:
			testing.ContextLogf(ctx, "Unknown tab priority %q", pri)
			return
		}
		if *recordLocation == nil {
			t, err := time.ParseInLocation(time.RFC3339Nano, timeString, time.UTC)
			if err != nil {
				testing.ContextLog(ctx, "Failed to parse time of discarded tab: ", err)
				return
			}
			*recordLocation = &TabDiscardInfo{t}
			testing.ContextLogf(ctx, "Tab discard observed, ID %d, %s", id, pri)
		}
	}
}

// NewTabDiscardObserver creates a new TabDiscardObserver
func NewTabDiscardObserver(ctx context.Context, cr *chrome.Chrome) (*TabDiscardObserver, error) {
	observeContext, cancel := context.WithCancel(ctx)
	cmd := testexec.CommandContext(observeContext, "tail", "-f", "-n", "0", cr.LogFilename())
	o := &TabDiscardObserver{cancel, make(map[int]string), nil, nil, nil}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get stdout for tailing chrome logs")
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start tailing chrome logs")
	}

	go func(ctx context.Context) {
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				o.Error = err
				return
			}
			o.observeLine(ctx, line)
		}
	}(observeContext)

	return o, nil
}
