// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package annotations

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// UserDirNetLogFile is the file path for netlog in user dir.
	// This file is present when chrome is started with the `--log-net-log` arg.
	UserDirNetLogFile string = "/home/chronos/netlog.json"

	// DownloadName is the file name for the default netlog.
	DownloadName string = "chrome-net-export-log.json"
)

// StartLogging clicks the "Start logging" button on the net export page.
func StartLogging(ctx context.Context, cr *chrome.Chrome, logRawBytes bool) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	netConn, err := NewNetExportConn(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "failed to load chrome://net-export")
	}
	defer netConn.Close()

	// Click Start over button in case this is not first use this session.
	if err := clickBtnOnPage(ctx, netConn, "startover"); err != nil {
		errors.Wrap(err, "failed to wait for the Start over")
	}

	// Perform cleanup in case a previous session was not cleaned up properly.
	// Deletes existing net log file(s), if they exist.
	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}
	logFile := filepath.Join(downloadsPath, DownloadName)
	if _, err := os.Stat(logFile); !errors.Is(err, fs.ErrNotExist) {
		testing.ContextLog(ctx, "A previous net log file exists = "+logFile)
		if err := os.Remove(logFile); err != nil {
			return errors.Wrap(err, "failed to delete existing net log file")
		}
	}

	// Select export type based on logRawBytes. If true, log with raw bytes.
	// If false, log with stripped private data.
	if logRawBytes {
		if err := clickBtnOnPage(ctx, netConn, "log-bytes-button"); err != nil {
			errors.Wrap(err, "failed to wait for the include raw bytes button to load")
		}
	} else {
		if err := clickBtnOnPage(ctx, netConn, "strip-private-data-button"); err != nil {
			errors.Wrap(err, "failed to wait for the strip private information button to load")
		}
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

// PrintAnnotationsAndCounts prints all the unique annotation hash codes
// and number of times that annotation occurred.
func PrintAnnotationsAndCounts(ctx context.Context, logFile []byte) {
	testing.ContextLog(ctx, "Printing the Annotations found in the file:")
	// Compile the regular expression.
	pattern := regexp.MustCompile(`\"traffic_annotation\":(\d+),`)

	// Find all matches of the regular expression.
	annotations := pattern.FindAllSubmatch(logFile, -1)

	// Create a map to store the unique annotations and their counts.
	annotationCounts := make(map[string]int)

	// Iterate over the annotations array.
	for _, annotation := range annotations {
		annotationCounts[string(annotation[1])]++
	}

	// Print the unique annotations and their counts.
	for annotationStr, count := range annotationCounts {
		testing.ContextLogf(ctx, "Annotation: %s: %d", annotationStr, count)
	}
}

// StopLoggingCheckLogs clicks the "Stop logging" button on the net export page and checks logs for given annotation.
func StopLoggingCheckLogs(ctx context.Context, cr *chrome.Chrome, annotation string) (foundAnnotation bool, err error) {
	// Open the net-export page.
	netConn, err := NewNetExportConn(ctx, cr)
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
	downloadLocation := filepath.Join(downloadsPath, DownloadName)

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

// StopLoggingVerifyAnnotationSet clicks the "Stop logging" button on the net export page and verifies that either none or all of the annotation hash codes in the given list are present in the logs.
func StopLoggingVerifyAnnotationSet(ctx context.Context, cr *chrome.Chrome, annotationsShouldBePresent bool, annotationHashCodes []string) (foundAnnotation bool, err error) {
	// Open the net-export page.
	netConn, err := NewNetExportConn(ctx, cr)
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

	return verifyAnnotationSet(downloadLocation, annotationsShouldBePresent, annotationHashCodes)
}

func verifyAnnotationSet(downloadLocation string, annotationsShouldBePresent bool, annotationHashCodes []string) (foundAnnotation bool, err error) {
	// Read the net export log file.
	logFile, err := ioutil.ReadFile(downloadLocation)
	if err != nil {
		return false, errors.Wrap(err, "failed to open logfile")
	}

	var oneAnnotationFound = false
	var annotationsFound []string
	for _, hashCode := range annotationHashCodes {
		// Check if the traffic annotation exists in the log file.
		annotationExists, err := regexp.Match(fmt.Sprintf("\"traffic_annotation\":%s", hashCode), logFile)
		oneAnnotationFound = annotationExists || oneAnnotationFound

		if err != nil {
			return oneAnnotationFound, errors.Wrap(err, "failed to search annotation logfile")
		}
		if annotationExists {
			annotationsFound = append(annotationsFound, hashCode)
		}
	}

	// Clean up file after reading.
	if err := os.Remove(downloadLocation); err != nil {
		return oneAnnotationFound, errors.Wrap(err, "failed to Clean file")
	}

	if !annotationsShouldBePresent {
		if len(annotationsFound) > 0 {
			return oneAnnotationFound, errors.Errorf("found unexpected annotations with the hash codes %+q", annotationsFound)
		}
	} else if len(annotationsFound) != len(annotationHashCodes) {
		return oneAnnotationFound, errors.Errorf("Did not find as many annotations as expected. Actual annotations found: %+q", annotationsFound)
	}
	return oneAnnotationFound, nil
}

// NewNetExportConn navigates to chrome://net-export.
func NewNetExportConn(ctx context.Context, cr *chrome.Chrome) (conn *chrome.Conn, err error) {
	// Open the net-export page.
	netConn, err := cr.NewConn(ctx, "chrome://net-export")
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
	downloadLocation := filepath.Join(downloadsPath, DownloadName)

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
func StopLogging(ctx context.Context, cr *chrome.Chrome) error {
	// Open the net-export page.
	netConn, err := NewNetExportConn(ctx, cr)
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
	downloadLocation := filepath.Join(downloadsPath, DownloadName)

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

// PollMultipleAnnotation checks collection of hashcodes in net-log.
// If annotation is found in net-log then status of annotation is updated
// to True in hashmap.
// Return annotation status map and error from polling.
func PollMultipleAnnotation(ctx context.Context, cr *chrome.Chrome, timeout, interval time.Duration, pollHashCodes []string) (map[string]bool, error) {
	// Map captures presence of required annotations in net-log.
	var hcLogStatus = make(map[string]bool)

	for _, hc := range pollHashCodes {
		hcLogStatus[hc] = false
	}

	// Wait to get annotation written to log.
	startTime := time.Now()
	err := testing.Poll(ctx,
		func(ctx context.Context) (err error) {
			for hc, found := range hcLogStatus {
				// If annotation is not found already, check for annotation in net-log.
				if !found {
					exists, errorCheckingLogs := CheckLogs(ctx, cr, hc)
					// Break the poll if error has been encountered while
					// checking net-log.
					if errorCheckingLogs != nil {
						return testing.PollBreak(errorCheckingLogs)
					}

					hcLogStatus[hc] = exists
				}
			}

			// When timeout has reached and errors have not been encountered so far.
			// Return without error to end the polling.
			if time.Since(startTime) >= timeout {
				return nil
			}

			// Return with error so that polling can continue
			// if all required annotations have not been found yet.
			for _, found := range hcLogStatus {
				if !found {
					return errors.New("not all annotations were found")
				}
			}

			// Return without error if all required annotations have been found
			// and no further polling is required.
			return nil
		}, &testing.PollOptions{
			Interval: interval,
		})

	// Return the status of logs and any error encountered.
	return hcLogStatus, err
}
