// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package actionlogger contains implementations to collect UI action logs.
package actionlogger

import (
	"context"
	"sync"

	alcommon "go.chromium.org/tast-tests/cros/common/actionlogger"
	"go.chromium.org/tast/core/errors"
)

// ActionLogger is the logger to collect UI action logs.
type ActionLogger struct {
	shouldRun bool
}

var (
	logger *ActionLogger
	once   sync.Once
)

func newActionLogger() *ActionLogger {
	return &ActionLogger{
		shouldRun: alcommon.ShouldRun.Value() == "true",
	}
}

// recordAction is to be called at the beginning of each UI action.
func (logger *ActionLogger) recordAction(ctx context.Context) {
}

// save saves all log info to the output dir, it is to be called at the end of each test.
func (logger *ActionLogger) save(ctx context.Context) {
}

// Reset clears the action item list.
func Reset() {
	if !logger.shouldRun {
		return
	}
	once.Do(func() {
		logger = newActionLogger()
	})
}

// Save saves the action logs.
func Save(ctx context.Context) error {
	if !logger.shouldRun {
		return nil
	}

	if logger != nil {
		return errors.New("action logger hasn't been initialized, please call Reset() before use")
	}
	logger.save(ctx)
	return nil

}

// RecordClickAction is for mouse click or touchscreen tap actions.
// Assume the bounding box and point are in DPI.
func RecordClickAction(ctx context.Context) {
	if !logger.shouldRun {
		return
	}

	logger.recordAction(ctx)
}
