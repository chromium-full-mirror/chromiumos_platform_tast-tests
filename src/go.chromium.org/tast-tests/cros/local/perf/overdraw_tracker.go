// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// OverdrawTracker is the helper to get overdraw data from Chrome.
type OverdrawTracker struct {
	collecting bool
}

// OverdrawData holds the average overdraw values.
type OverdrawData struct {
	AverageOverdraws []float64 `json:"averageOverdraws"`
}

// Start starts overdraw tracking. This can only be called if there we are not
// currently tracking overdraw.
func (t *OverdrawTracker) Start(ctx context.Context, tconn *chrome.TestConn, bucketSize int) error {
	if t.collecting {
		return errors.New("overdraw tracking already started")
	}
	t.collecting = true

	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.startOverdrawTracking)`, bucketSize); err != nil {
		return errors.Wrap(err, "failed to start overdraw collection")
	}

	return nil
}

// Stop stops overdaw tracking. This can only be called after |Start| is
// called.
func (t *OverdrawTracker) Stop(ctx context.Context, tconn *chrome.TestConn) (*OverdrawData, error) {
	if !t.collecting {
		return nil, errors.New("overdraw tracking hasn't started yet")
	}
	t.collecting = false

	var overdrawData OverdrawData
	if err := tconn.Call(ctx, &overdrawData, `tast.promisify(chrome.autotestPrivate.stopOverdrawTracking)`); err != nil {
		return nil, errors.Wrap(err, "failed to start overdraw collection")
	}
	return &overdrawData, nil
}

// NewOverdrawTracker creates a new instance of the OverdrawTracker.
func NewOverdrawTracker() *OverdrawTracker {
	return &OverdrawTracker{}
}
