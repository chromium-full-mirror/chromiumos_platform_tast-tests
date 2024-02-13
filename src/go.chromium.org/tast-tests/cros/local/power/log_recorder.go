// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast/core/errors"
)

// LogRecorder records power related logs.
type LogRecorder struct {
	logMarkers map[string]*logsaver.Marker
	outDir     string
}

// NewLogRecorder returns a LogRecorder for recording power related logs.
func NewLogRecorder(outDir string) *LogRecorder {
	return &LogRecorder{logMarkers: make(map[string]*logsaver.Marker),
		outDir: outDir}
}

// Start recording logs.
func (lr *LogRecorder) Start() error {
	if len(lr.logMarkers) != 0 {
		return errors.New("Log markers were already created but not cleaned up")
	}

	logMarker, err := logsaver.NewMarker("/var/log/cros_ec.log")
	if err != nil {
		return errors.Wrap(err, "failed to start the log saver")
	}

	lr.logMarkers["cros_ec.log"] = logMarker

	return nil
}

// Stop recording logs and write them to the given output directory.
func (lr *LogRecorder) Stop() error {
	for logName, logMarker := range lr.logMarkers {
		if err := logMarker.Save(filepath.Join(lr.outDir, logName)); err != nil {
			return errors.Wrap(err, "failed to store log data")
		}
	}

	return nil
}
