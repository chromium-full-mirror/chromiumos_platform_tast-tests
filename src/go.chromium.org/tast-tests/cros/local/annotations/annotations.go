// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package annotations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
)

const (
	// UserDirNetLogFile is the file path for netlog in user dir.
	// This file is present when chrome is started with the `--log-net-log` arg.
	UserDirNetLogFile string = "/home/chronos/netlog.json"
)

// StartLogging clicks the "Start logging" button on the net export page.
func StartLogging(ctx context.Context, cr *chrome.Chrome, br *browser.Browser) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	netConn, err := NewNetExportConn(ctx, br)
	if err != nil {
		return errors.Wrap(err, "failed to load chrome://net-export")
	}
	defer netConn.Close()

	// Click Start over button in case this is not first use this session.
	if err := clickBtnOnPage(ctx, netConn, "startover"); err != nil {
		errors.Wrap(err, "failed to wait for the Start over")
	}

	// Click Start Log button.
	if err := clickBtnOnPage(ctx, netConn, "start-logging"); err != nil {
		errors.Wrap(err, "failed to wait for the Start Logging button to load")
	}

	// Click Save button to choose the filename for log file.
	ui := uiauto.New(tconn)
	saveButton := nodewith.Name("Save").Role(role.Button)
	if err := uiauto.Combine("Click 'Save' button",
		ui.WaitUntilExists(saveButton),
		ui.WaitUntilEnabled(saveButton),
		ui.DoDefault(saveButton),
		ui.WaitUntilGone(saveButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to complete save file steps")
	}
	return nil
}

// StopLoggingCheckLogs clicks the "Stop logging" button on the net export page and checks logs for given annotation.
func StopLoggingCheckLogs(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, annotation string) (foundAnnotation bool, err error) {
	// Open the net-export page.
	netConn, err := NewNetExportConn(ctx, br)
	if err != nil {
		return false, errors.Wrap(err, "failed to load chrome://net-export")
	}
	defer netConn.Close()

	// Click Stop Logging button.
	if err := clickBtnOnPage(ctx, netConn, "stop-logging"); err != nil {
		return false, errors.Wrap(err, "failed to wait for the Stop Logging button to load")
	}

	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return false, errors.Wrap(err, "failed to get user's Download path")
	}
	downloadName := "chrome-net-export-log.json"
	downloadLocation := filepath.Join(downloadsPath, downloadName)

	// Read the net export log file.
	logFile, err := ioutil.ReadFile(downloadLocation)
	if err != nil {
		return false, errors.Wrap(err, "failed to open logfile")
	}
	// Check if the traffic annotation exists in the log file.
	isExist, err := regexp.Match(fmt.Sprintf("\"traffic_annotation\":%s", annotation), logFile)
	if err != nil {
		return false, errors.Wrap(err, "failed to search annotation logfile")
	}

	// Clean up file after reading
	if err := os.Remove(downloadLocation); err != nil {
		return false, errors.Wrap(err, "failed to Clean file")
	}

	return isExist, nil
}

// StopLoggingVerifyNoAnnotation clicks the "Stop logging" button on the net export page and verifies that none of the annotation hash codes in the given list are present in the logs.
func StopLoggingVerifyNoAnnotation(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, annotationHashCodes []string) (foundAnnotation bool, err error) {
	// Open the net-export page.
	netConn, err := NewNetExportConn(ctx, br)
	if err != nil {
		return false, errors.Wrap(err, "failed to load chrome://net-export")
	}
	defer netConn.Close()

	// Click Stop Logging button.
	if err := clickBtnOnPage(ctx, netConn, "stop-logging"); err != nil {
		return false, errors.Wrap(err, "failed to wait for the Stop Logging button to load")
	}

	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return false, errors.Wrap(err, "failed to get user's Download path")
	}
	downloadName := "chrome-net-export-log.json"
	downloadLocation := filepath.Join(downloadsPath, downloadName)

	// Read the net export log file.
	logFile, err := ioutil.ReadFile(downloadLocation)
	if err != nil {
		return false, errors.Wrap(err, "failed to open logfile")
	}

	var annotationsFound []string
	for _, hashCode := range annotationHashCodes {
		// Check if the traffic annotation exists in the log file.
		annotationExists, err := regexp.Match(fmt.Sprintf("\"traffic_annotation\":%s", hashCode), logFile)
		if err != nil {
			return false, errors.Wrap(err, "failed to search annotation logfile")
		}
		if annotationExists {
			annotationsFound = append(annotationsFound, hashCode)
		}
	}

	// Clean up file after reading.
	if err := os.Remove(downloadLocation); err != nil {
		return false, errors.Wrap(err, "failed to Clean file")
	}

	if len(annotationsFound) > 0 {
		return false, errors.Errorf("found unexpected annotations with the hash codes %+q", annotationsFound)
	}
	return true, nil
}

// NewNetExportConn navigates to chrome://net-export.
func NewNetExportConn(ctx context.Context, br *browser.Browser) (conn *chrome.Conn, err error) {
	// Open the net-export page.
	netConn, err := br.NewConn(ctx, "chrome://net-export")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load chrome://net-export")
	}
	return netConn, nil
}

// CheckLogs checks logs for given annotation.
func CheckLogs(ctx context.Context, cr *chrome.Chrome, annotation string) (foundAnnotation bool, err error) {
	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return false, errors.Wrap(err, "failed to get user's Download path")
	}
	downloadName := "chrome-net-export-log.json"
	downloadLocation := filepath.Join(downloadsPath, downloadName)

	return CheckLogsFromFile(ctx, cr, annotation, downloadLocation)
}

// CheckLogsFromFile checks logs for given annotation in given file.
func CheckLogsFromFile(ctx context.Context, cr *chrome.Chrome, annotation, logFilePath string) (foundAnnotation bool, err error) {
	// Read the net export log file.
	logFile, err := ioutil.ReadFile(logFilePath)
	if err != nil {
		return false, errors.Wrap(err, "failed to open logfile")
	}
	// Check if the traffic annotation exists in the log file.
	isExist, err := regexp.Match(fmt.Sprintf("\"traffic_annotation\":%s", annotation), logFile)
	if err != nil {
		return false, errors.Wrap(err, "failed to search annotation logfile")
	}

	return isExist, nil
}

// StopLogging clicks the "Stop logging" button and deletes the logs.
func StopLogging(ctx context.Context, cr *chrome.Chrome, br *browser.Browser) error {
	// Open the net-export page.
	netConn, err := NewNetExportConn(ctx, br)
	if err != nil {
		return errors.Wrap(err, "failed to load chrome://net-export")
	}
	defer netConn.Close()

	// Click Stop Logging button.
	if err := clickBtnOnPage(ctx, netConn, "stop-logging"); err != nil {
		return errors.Wrap(err, "failed to wait for the Stop Logging button to load")
	}

	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}
	downloadName := "chrome-net-export-log.json"
	downloadLocation := filepath.Join(downloadsPath, downloadName)

	// Clean up file after reading
	if err := os.Remove(downloadLocation); err != nil {
		return errors.Wrap(err, "failed to Clean file")
	}

	return nil
}

// clickBtnOnPage helper function to click on buttons on the page.
func clickBtnOnPage(ctx context.Context, netConn *chrome.Conn, btnID string) error {
	btn := `document.getElementById("` + btnID + `")`
	if err := netConn.WaitForExpr(ctx, btn); err != nil {
		return errors.Wrap(err, "failed to find btn:"+btnID)
	}
	if err := netConn.Eval(ctx, btn+`.click()`, nil); err != nil {
		return errors.Wrap(err, "failed to click btn:"+btnID)
	}

	return nil
}
