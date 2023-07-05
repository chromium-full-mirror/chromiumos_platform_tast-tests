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
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// UserDirNetLogFile is the file path for netlog in user dir.
	// This file is present when chrome is started with the `--log-net-log` arg.
	UserDirNetLogFile string = "/home/chronos/netlog.json"

	// DownloadName is the file name for the default netlog.
	DownloadName string = "chrome-net-export-log.json"
	// OsDownloadName is a custom file name used for the OS netlog in lacros. This
	// prevents naming conflicts when using both OS and browser netlogs in the
	// same lacros session.
	OsDownloadName string = "chrome-os-net-export-log.json"

	// OsNetExportURL is the URL for the OS net-export app in lacros.
	OsNetExportURL string = "os://net-export"
)

// StartLogging clicks the "Start logging" button on the net export page.
func StartLogging(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, logRawBytes bool) error {
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

// StartOSLogging starts a net export session via the os://net-export app. This
// creates a JSON file in the user's Downloads folder which contains network
// activity that is observed from the system's NetworkService while the session
// is active. This should only be used in lacros mode, since lacros splits the
// browser NetworkService from the OS (Ash) NetworkService. Annotations present
// in the browser (Lacros) binary, and all annotations while running in Ash
// mode, should be checked using `StartLogging(...)` instead.
func StartOSLogging(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, kb *input.KeyboardEventWriter) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	netConn, err := NewOSNetExportConn(ctx, br, tconn, kb)
	if err != nil {
		return errors.Wrap(err, "failed to load "+OsNetExportURL)
	}
	defer netConn.Close()

	netExportWindow := nodewith.Name("ChromeOS-URLs - Network Log Export").Role(role.Window)
	resetButton := nodewith.Name("Start Over").Role(role.Button)
	startButton := nodewith.Name("Start Logging to Disk").Role(role.Button)
	saveButton := nodewith.Name("Save").Role(role.Button)
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Start net export session",
		ui.WaitUntilExists(netExportWindow),
		// Click 'Start over' button if present. This is necessary to start any
		// net export session other than the first one in a single chrome session.
		uiauto.IfSuccessThen(
			ui.WithTimeout(3*time.Second).WaitUntilExists(resetButton),
			ui.DoDefault(resetButton)),
		// Click 'Start' button to begin net export session.
		ui.WaitUntilExists(startButton),
		ui.DoDefault(startButton),
		// Set file name for netlog file.
		setFileName(OsDownloadName, ui, kb),
		// Click 'Save' button in file app.
		ui.WaitUntilExists(saveButton),
		ui.WaitUntilEnabled(saveButton),
		ui.DoDefault(saveButton),
		ui.WaitUntilGone(saveButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to start net export session")
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

// StopOSLoggingCheckLogs stops a previously started OS net export session via
// the os://net-export app, and returns true if the given network annotation was
// present in the net log file.
func StopOSLoggingCheckLogs(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, kb *input.KeyboardEventWriter, annotation string) (foundAnnotation bool, err error) {
	if err := stopOSLogging(ctx, cr, br, kb); err != nil {
		return false, errors.Wrap(err, "failed to stop logging")
	}

	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return false, errors.Wrap(err, "failed to get user's Download path")
	}
	downloadLocation := filepath.Join(downloadsPath, OsDownloadName)

	// Check file for annotation.
	foundAnnotation, err = CheckLogsFromFile(ctx, cr, annotation, downloadLocation)
	if err != nil {
		return false, errors.Wrap(err, "failed to check net log file for annotation")
	}

	// Clean up file after reading.
	if err := os.Remove(downloadLocation); err != nil {
		return false, errors.Wrapf(err, "failed to remove file: %s", downloadLocation)
	}

	return foundAnnotation, nil
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

// StopOSLoggingVerifyAnnotationSet clicks the "Stop logging" button on the net
// export page and verifies that either none or all of the annotation hash codes
// in the given list are present in the logs.
func StopOSLoggingVerifyAnnotationSet(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, kb *input.KeyboardEventWriter, annotationsShouldBePresent bool, annotationHashCodes []string) (foundAnnotation bool, err error) {
	if err := stopOSLogging(ctx, cr, br, kb); err != nil {
		return false, errors.Wrap(err, "failed to stop logging")
	}

	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return false, errors.Wrap(err, "failed to get user's Download path")
	}
	downloadLocation := filepath.Join(downloadsPath, OsDownloadName)

	return verifyAnnotationSet(downloadLocation, annotationsShouldBePresent, annotationHashCodes)
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

// NewOSNetExportConn navigates to os://net-export. Note: This app only exists
// in Lacros mode.
func NewOSNetExportConn(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) (conn *chrome.Conn, err error) {
	// Connect to a new tab.
	conn, err = br.NewConn(ctx, chrome.NewTabURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find a new tab page")
	}
	defer conn.Close()

	// We cannot use "conn.Navigate(ctx, url)" here, as that does not use the
	// omnibox navigation which should be used to get re-routed. As such we have
	// to enter the navigation into the omnibox to navigate.
	// Source: lacros.URLRedirect
	ui := uiauto.New(tconn)
	return conn, uiauto.Combine("open target "+OsNetExportURL,
		ui.LeftClick(browserui.AddressBarFinder),
		kb.AccelAction("ctrl+a"),
		kb.TypeAction(OsNetExportURL),
		kb.AccelAction("Enter"))(ctx)
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

// CheckOSLogs checks OS logs for given annotation.
func CheckOSLogs(ctx context.Context, cr *chrome.Chrome, annotation string) (foundAnnotation bool, err error) {
	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return false, errors.Wrap(err, "failed to get user's Download path")
	}
	downloadLocation := filepath.Join(downloadsPath, OsDownloadName)

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

// StopOSLogging clicks the "Stop logging" button and deletes the logs.
func StopOSLogging(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, kb *input.KeyboardEventWriter) error {
	if err := stopOSLogging(ctx, cr, br, kb); err != nil {
		return errors.Wrap(err, "failed to stop OS logging")
	}

	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}
	downloadLocation := filepath.Join(downloadsPath, OsDownloadName)

	// Clean up file after reading.
	if err := os.Remove(downloadLocation); err != nil {
		return errors.Wrap(err, "failed to Clean file")
	}

	return nil
}

func stopOSLogging(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, kb *input.KeyboardEventWriter) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	// Open the net-export page.
	netConn, err := NewOSNetExportConn(ctx, br, tconn, kb)
	if err != nil {
		return errors.Wrap(err, "failed to load "+OsNetExportURL)
	}
	defer netConn.Close()

	stopButton := nodewith.Name("Stop Logging").Role(role.Button)
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Stop net export session",
		ui.WaitUntilExists(stopButton),
		ui.DoDefault(stopButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to stop net export session")
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

// setFileName sets the file name in file app during the save file process.
// This function uses polling with retries and verification because this process
// is flaky and can result in incorrectly set file names.
func setFileName(desiredFileName string, ui *uiauto.Context, kb *input.KeyboardEventWriter) uiauto.Action {
	fileNameField := nodewith.Name("File name").Role(role.TextField).State(state.Focused, true)

	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			if err := uiauto.Combine("Change file name in file picker",
				ui.WaitUntilExists(fileNameField),
				kb.AccelAction("ctrl+a"),
				kb.TypeAction(desiredFileName),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to change file name")
			}

			fileNameFieldInfo, err := ui.Info(ctx, fileNameField)
			if err != nil {
				return errors.Wrap(err, "failed to read file name")
			}
			if fileNameFieldInfo.Value != desiredFileName {
				errMsg := fmt.Sprintf("file name was not renamed correctly = got %s, want %s", fileNameFieldInfo.Value, desiredFileName)
				testing.ContextLog(ctx, errMsg)
				return errors.Wrap(err, errMsg)
			}
			return nil
		}, &testing.PollOptions{Interval: time.Second, Timeout: 15 * time.Second})
	}
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
