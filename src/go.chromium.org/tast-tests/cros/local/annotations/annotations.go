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
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
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

// StopLoggingCheckLogsFilterByTriggerTime clicks the "Stop logging" button on the net export page and checks logs for given annotation and filter out annotations before trigger time.
func StopLoggingCheckLogsFilterByTriggerTime(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, annotation string, triggerTime time.Time) (foundAnnotation bool, err error) {
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

	// Check if annotation timestamps are after trigger.
	isValid, err := CheckAnnotationTimes(annotation, logFile, triggerTime)
	if err != nil {
		return false, errors.Wrap(err, "failed to check annotation timestamps")
	}

	// Clean up file after reading.
	if err := os.Remove(downloadLocation); err != nil {
		return false, errors.Wrap(err, "failed to clean file")
	}

	// Return true if annotation exists and occurs after trigger.
	return isExist && isValid, nil
}

// StopLoggingVerifyAnnotationSet clicks the "Stop logging" button on the net export page and verifies that either none or all of the annotation hash codes in the given list are present in the logs.
func StopLoggingVerifyAnnotationSet(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, annotationsShouldBePresent bool, annotationHashCodes []string) (foundAnnotation bool, err error) {
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

// CheckAnnotationTimes checks validity of annotations by comparing an input trigger time with the annotation times.
// Returns true if and only if an annotation exists after the trigger time.
func CheckAnnotationTimes(annotation string, logFile []byte, triggerTime time.Time) (bool, error) {
	annotationTimes, err := GetAnnotationTimes(annotation, logFile)
	if err != nil {
		return false, err
	}
	for _, annotationTime := range annotationTimes {
		if triggerTime.Before(annotationTime) {
			return true, nil
		}
	}
	return false, nil
}

// GetAnnotationTimes checks a log file and returns the annotation times in time.Time objects.
func GetAnnotationTimes(annotation string, logFile []byte) (realTimes []time.Time, err error) {
	startTimeTick, err := GetStartTimeTick(logFile)
	if err != nil {
		return nil, err
	}

	// Get annotation offset times.
	var annotationTimeTicks []int
	annotationPrefix := fmt.Sprintf("\"traffic_annotation\":%s", annotation)
	re := regexp.MustCompile(fmt.Sprintf(`%s.*\"time\":\"(\d+)`, annotationPrefix))
	matches := re.FindAllSubmatch(logFile, -1)
	for _, match := range matches {
		matchTimeTick, err := strconv.Atoi(string(match[1]))
		if err == nil {
			annotationTimeTicks = append(annotationTimeTicks, matchTimeTick)
		}
	}

	// Calculate annotation times.
	for _, annotationTimeTick := range annotationTimeTicks {
		timeTick := startTimeTick + annotationTimeTick
		realTime := time.Unix(0, int64(timeTick)*int64(time.Millisecond))
		realTimes = append(realTimes, realTime)
	}
	return realTimes, nil
}

// GetStartTimeTick gets the log start time in milliseconds. If timeTickOffset is not found, returns 0 with error.
func GetStartTimeTick(logFile []byte) (int, error) {
	re := regexp.MustCompile(`timeTickOffset\":(\d+)`)
	matches := re.FindSubmatch(logFile)
	if matches == nil {
		return 0, errors.New("failed to find timeTickOffset in logfile")
	}
	return strconv.Atoi(string(matches[1]))
}
