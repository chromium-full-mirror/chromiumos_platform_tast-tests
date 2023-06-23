// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package netexport is a test helper for managing net export sessions in
// tests, and reading annotations in the resulting net log file(s). This allows
// tests to track what network calls are made during the test run.
// Example usage:
//
//	netExport := netexport.Start(ctx, cr, br, s.Param().(browser.Type))
//	defer netExport.Cleanup(cleanupCtx) // Stop net export and delete logfile.
//
//	<your test performs actions to trigger network calls>
//
//	foundAnnotation, err := netExport.Find('annotation123')
//	  or
//	foundAnnotation, err := netExport.FindUntil('annotation123',
//	    &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second})
package netexport

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Start starts a net export session using either:
//   - chrome://net-export (Ash) or
//   - chrome://net-export and os://net-export (Lacros).
func Start(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, bt browser.Type) (NetExport, error) {
	// Get user's Download folder path. The net log file is stored in this folder.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return NetExport{}, errors.Wrap(err, "failed to get user's Download path")
	}

	var netExport NetExport = NetExport{}
	if bt == browser.TypeLacros {
		// Set up keyboard. Cleanup is handled in `osController.Stop()`.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return NetExport{}, errors.Wrap(err, "failed to setup keyboard")
		}

		// Use both `os://net-export` and `chrome://net-export`.
		netExport = NetExport{
			logFiles: []string{
				filepath.Join(downloadsPath, annotations.OsDownloadName),
				filepath.Join(downloadsPath, annotations.DownloadName)},
			controllers: []controller{
				osController{ctx, cr, br, kb},
				chromeController{ctx, cr, br}},
		}
	} else {
		// Use `chrome://net-export`.
		netExport = NetExport{
			logFiles:    []string{filepath.Join(downloadsPath, annotations.DownloadName)},
			controllers: []controller{chromeController{ctx, cr, br}},
		}
	}

	// Perform cleanup in case a previous session was not cleaned up properly.
	// Deletes existing net log file(s), if they exist. Note: This does not call
	// `controller.Stop()` since sessions are automatically stopped when the net
	// export page is closed.
	for _, logFile := range netExport.logFiles {
		if _, err := os.Stat(logFile); err == nil {
			testing.ContextLog(ctx, "A previous net log file exists = "+logFile)
			if err := os.Remove(logFile); err != nil {
				return netExport, errors.Wrap(err, "failed to delete existing net log file")
			}
		}
	}

	for _, controller := range netExport.controllers {
		if err := controller.Start(); err != nil {
			return netExport, errors.Wrap(err, "failed to start net export")
		}
	}
	return netExport, nil
}

// FromCommandLineArg should be used when Chrome is started with the arg:
// `--log-net-log`. This means that the net export session is already
// started and cannot be stopped, and the net log file already exists.
// TODO(b/289435114): Extend this to support the Lacros binary.
func FromCommandLineArg() NetExport {
	return NetExport{
		logFiles: []string{annotations.UserDirNetLogFile},
		// Set controllers to nil, since the net export session is already started.
		controllers: nil,
	}
}

// NetExport is the object returned after starting a net export session. This
// object provides helper functions for finding annotations in the net log
// file(s).
type NetExport struct {
	// logFiles is a list of file paths to the JSON netlog files created as a
	// result of the net export session. Note that in lacros mode, there are
	// two log files, one for each binary. When searching through these files for
	// network annotations, we read through all files and treat them as one.
	logFiles []string
	// controllers is responsible for starting and stopping the net export
	// session. This is extracted to an interface because different net exports,
	// such as chrome://net-export vs os://net-export, are started and stopped in
	// different ways, but the log files are formatted and read in the same way.
	controllers []controller
}

// Find searches for the given annotationID string in the log file(s). Returns
// true when one or more occurrence is found in any file.
func (ne *NetExport) Find(annotationID string) (foundAnnotation bool, err error) {
	for _, logFile := range ne.logFiles {
		// Read the net export log file.
		logFile, err := ioutil.ReadFile(logFile)
		if err != nil {
			return false, errors.Wrap(err, "failed to open logfile")
		}
		// Check if the traffic annotation exists in the log file.
		isExist, err := regexp.Match(fmt.Sprintf("\"traffic_annotation\":%s", annotationID), logFile)
		if err != nil {
			return false, errors.Wrap(err, "failed to search annotation logfile")
		}

		if isExist {
			return true, nil
		}
	}

	return false, nil
}

// FindUntil is similar to Find, however it polls for the annotationID for the
// given duration. This is recommended for when the network call may occur
// asynchronously within the test.
func (ne *NetExport) FindUntil(ctx context.Context, annotationID string, opts *testing.PollOptions) (foundAnnotation bool, err error) {
	var errorCheckingLogs error
	var isFound bool
	testing.Poll(ctx, func(ctx context.Context) (err error) {
		// Check the logs for given annotation.
		isFound, errorCheckingLogs = ne.Find(annotationID)

		// Found annotation. Return nil to end polling.
		if isFound {
			return nil
		}

		return errors.New("Annotation ID not found yet")
	}, opts)

	return isFound, errorCheckingLogs
}

// Note: We can easily add new methods here since this is decoupled from
// start/stop logging:
//   * FindSet(['annotationID1', 'annotationID2'])
//   * FindAfterTimestamp(...)

// Cleanup stops the net export session, and deletes the related log file(s). It
// should generally be called via a defer function immediately after starting
// the net export session, and should use the cleanupCtx from the test. Example:
//
// netExport := netexport.Start(ctx, cr, br)
// defer netExport.Cleanup(cleanupCtx) // Stop net export and delete logfile.
func (ne *NetExport) Cleanup(ctx context.Context) {
	if ne.controllers == nil {
		return // Nothing to clean up.
	}

	// Stop net export session via the same *://net-export page(s) used to start
	// it. Note: This call may not be strictly necessary since the session will be
	// stopped after the net export page is closed. We include it for now just to
	// be sure that everything is stopped and cleaned up as expected.
	for _, controller := range ne.controllers {
		if err := controller.Stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop net export session: ", err)
		}
	}

	// Delete net log file(s).
	for _, logFile := range ne.logFiles {
		if err := os.Remove(logFile); err != nil {
			testing.ContextLog(ctx, "Failed to delete net log file: ", err)
		}
	}
}
