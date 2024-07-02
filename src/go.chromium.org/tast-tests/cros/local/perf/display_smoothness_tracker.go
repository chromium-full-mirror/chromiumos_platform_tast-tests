// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// DisplaySmoothnessTracker helps to start/stop display smoothness tracking.
type DisplaySmoothnessTracker struct {
	// Ids of the displays that have display smoothness tracked.
	displayIDs map[string]bool
}

// DisplayFrameData holds the collected display frame data.
type DisplayFrameData struct {
	StartOffsetMs  int       `json:"startOffsetMs"`
	StopOffsetMs   int       `json:"stopOffsetMs"`
	FramesExpected int       `json:"framesExpected"`
	FramesProduced int       `json:"framesProduced"`
	JankCount      int       `json:"jankCount"`
	Throughput     []int     `json:"throughput"`
	JankDurations  []float64 `json:"jankDurations"`
}

// displayIDString returns a string representing the given display id.
func displayIDString(displayID string) string {
	if displayID == "" {
		return "primary display"
	}
	return displayID
}

// Close ensures all started tracking is stopped.
func (t *DisplaySmoothnessTracker) Close(ctx context.Context, tconn *chrome.TestConn) error {
	var firstErr error
	for displayID := range t.displayIDs {
		_, err := t.Stop(ctx, tconn, displayID)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Start starts tracking for the given display id. Primary display is used
// if the given display id is empty.
func (t *DisplaySmoothnessTracker) Start(ctx context.Context, tconn *chrome.TestConn, displayID string, interval time.Duration) error {
	_, found := t.displayIDs[displayID]
	if found {
		return errors.Errorf("display smoothness already tracked for %q", displayIDString(displayID))
	}

	err := tconn.Call(ctx, nil,
		`tast.promisify(chrome.autotestPrivate.startSmoothnessTracking)`, displayID, interval.Milliseconds())
	if err != nil {
		return err
	}

	t.displayIDs[displayID] = true
	return nil
}

// Stop stops tracking for the given display id and report the smoothness
// since the relevant Start() call. Primary display is used if the given display
// id is empty.
func (t *DisplaySmoothnessTracker) Stop(ctx context.Context, tconn *chrome.TestConn, displayID string) (*DisplayFrameData, error) {
	// If there is existing smoothness tracking for the display, we need to
	// clear it even if |t| doesn't know about it. Thus, call the API first
	// to clear whatever existing tracking is there, and then do the check
	// if |t| knows about any display smoothness tracking for |displayID|.
	var dsData DisplayFrameData
	err := tconn.Call(ctx, &dsData,
		`tast.promisify(chrome.autotestPrivate.stopSmoothnessTracking)`, displayID)
	if err != nil {
		return nil, err
	}

	_, found := t.displayIDs[displayID]
	if !found {
		return nil, errors.Errorf("display smoothness not tracked for %q", displayIDString(displayID))
	}

	delete(t.displayIDs, displayID)
	return &dsData, nil
}

// NewDisplaySmoothnessTracker creates a DisplaySmoothnessTracker.
func NewDisplaySmoothnessTracker() *DisplaySmoothnessTracker {
	return &DisplaySmoothnessTracker{
		displayIDs: map[string]bool{},
	}
}
