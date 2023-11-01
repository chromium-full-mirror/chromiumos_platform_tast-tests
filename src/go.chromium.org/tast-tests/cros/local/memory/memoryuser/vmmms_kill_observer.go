// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"bufio"
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VmmmsKillInfo describes a kill observed by the VM Memory Management Service.
type VmmmsKillInfo struct {
	Time time.Time
}

// VmmmsKillObserver monitors /var/log/messages to record the time that the
// VM Memory Management Service observes tab discards and app kills.
type VmmmsKillObserver struct {
	cancel context.CancelFunc

	// The first kill event of the given priority that occurred after this
	// observer was constructed or reset.
	CachedApp      *VmmmsKillInfo
	CachedTab      *VmmmsKillInfo
	PerceptibleApp *VmmmsKillInfo
	PerceptibleTab *VmmmsKillInfo
	FocusedApp     *VmmmsKillInfo

	// The most recent error encountered by this observer.
	Error error
}

// Close causes this observer to stop monitoring for new kill events.
func (o *VmmmsKillObserver) Close() {
	o.cancel()
}

// AllPrioritiesObserved returns true if all priorities of kills have been
// observed and they have a timestamp.
func (o *VmmmsKillObserver) AllPrioritiesObserved() bool {
	return o.CachedApp != nil && o.CachedTab != nil && o.PerceptibleApp != nil && o.PerceptibleTab != nil && o.FocusedApp != nil
}

// 2023-10-31T07:32:23.450025Z INFO vm_concierge[21465]: KillTrace:[35,RESIZE_PRIORITY_CACHED_APP,53MB]
var killTraceRE = regexp.MustCompile(`^(?P<timestamp>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}(?:Z|[+-]\d{2}:\d{2})) INFO vm_concierge\[\d+\]: KillTrace:\[(?P<cid>\d+),(?P<priority>[A-Z_]+),(?P<sizeMB>\d+)MB\]`)

func (o *VmmmsKillObserver) observeLine(ctx context.Context, line string) {
	m := killTraceRE.FindStringSubmatch(line)
	if m == nil {
		return
	}
	ts, err := time.Parse(time.RFC3339Nano, m[1])
	if err != nil {
		testing.ContextLogf(ctx, "Invalid timestamp for VMMMS kill log line %q", m[1])
		return
	}

	priority := m[3]
	var recordLocation **VmmmsKillInfo
	switch priority {
	case "RESIZE_PRIORITY_FOCUSED_APP":
		recordLocation = &o.FocusedApp
	case "RESIZE_PRIORITY_PERCEPTIBLE_TAB":
		recordLocation = &o.PerceptibleTab
	case "RESIZE_PRIORITY_PERCEPTIBLE_APP":
		recordLocation = &o.PerceptibleApp
	case "RESIZE_PRIORITY_CACHED_TAB":
		recordLocation = &o.CachedTab
	case "RESIZE_PRIORITY_CACHED_APP":
		recordLocation = &o.CachedApp
	default:
		testing.ContextLogf(ctx, "Unknown VMMMS kill priority %q", priority)
		return
	}

	if *recordLocation == nil {
		*recordLocation = &VmmmsKillInfo{ts}
		testing.ContextLogf(ctx, "VMMMS kill observed, %s, %s, %sMB", m[2], m[3], m[4])
	}
}

// NewVmmmmsKillObserver creates a new VmmmsKillObserver
func NewVmmmmsKillObserver(ctx context.Context) (*VmmmsKillObserver, error) {
	observeContext, cancel := context.WithCancel(ctx)
	o := &VmmmsKillObserver{cancel, nil, nil, nil, nil, nil, nil}
	cmd := testexec.CommandContext(observeContext, "tail", "-f", "-n", "0", "/var/log/messages")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get stdout for tailing /var/log/messages")
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start tailing /var/log/messages")
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
