// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package helpers contains helper functions for enterprise connector tests.
package helpers

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParams entail parameters describing the set policy for a user and more.
type TestParams struct {
	AllowsImmediateDelivery bool         // Specifies whether immediate delivery of files is allowed.
	AllowsUnscannableFiles  bool         // Specifies whether unscannable files (large or encrypted) are allowed.
	ScansEnabled            bool         // Specifies whether malware and dlp scans are enabled.
	BrowserType             browser.Type // Specifies the type of browser.
}

// TestFileParams describe the parameters for a test file.
type TestFileParams struct {
	TestName      string
	FileName      string
	UlBlockLabel  string
	IsBad         bool
	IsUnscannable bool
	IsWarn        bool
}

// DownloadBubbleState indicates the download state indicated by the download bubble (see launch/4200678).
type DownloadBubbleState string

const (
	// DownloadBubbleStateUnavailable means that the download bubble UI wasn't found, so the feature is unavailable.
	DownloadBubbleStateUnavailable DownloadBubbleState = "unavailable"
	// DownloadBubbleStateAllowed means that a download was allowed.
	DownloadBubbleStateAllowed DownloadBubbleState = "allowed"
	// DownloadBubbleStateBlocked means that a download was blocked.
	DownloadBubbleStateBlocked DownloadBubbleState = "blocked"
)

// ScanningTimeOut describes the typical time out for a scan.
// The scanning timeout of chrome is 5 minutes, so we wait a bit more to get a proper TIMEOUT notification.
const ScanningTimeOut = 6 * time.Minute

// FcmTokenTimeOut describes how long we wait for a valid fcm token.
const FcmTokenTimeOut = 10 * time.Minute

// GetTestFileParams returns the list of parameters for the files that should be tested.
func GetTestFileParams() []TestFileParams {
	return []TestFileParams{
		{
			TestName:      "Encrypted malware",
			FileName:      "unknown_malware_encrypted.zip",
			UlBlockLabel:  "This file is encrypted. Ask its owner to decrypt.",
			IsBad:         true,
			IsUnscannable: true,
			IsWarn:        false,
		},
		{
			TestName:      "Unknown malware",
			FileName:      "unknown_malware.zip",
			UlBlockLabel:  "This file or your device doesn’t meet some of your organization’s security policies. Check with your admin on what needs to be fixed.",
			IsBad:         true,
			IsUnscannable: false,
			IsWarn:        false,
		},
		{
			TestName:      "Known malware",
			FileName:      "content.exe",
			UlBlockLabel:  "This file or your device doesn’t meet some of your organization’s security policies. Check with your admin on what needs to be fixed.",
			IsBad:         true,
			IsUnscannable: false,
			IsWarn:        false,
		},
		{
			TestName:      "DLP clear text",
			FileName:      "10ssns.txt",
			UlBlockLabel:  "This file or your device doesn’t meet some of your organization’s security policies. Check with your admin on what needs to be fixed.",
			IsBad:         true,
			IsUnscannable: false,
			// 10ssns.txt also triggers the warning rule, so expect both a warn and a block verdict.
			IsWarn: true,
		},
		{
			TestName:      "Allowed file",
			FileName:      "allowed.txt",
			UlBlockLabel:  "",
			IsBad:         false,
			IsUnscannable: false,
			IsWarn:        false,
		},
	}
}

// GetTestFileParamsWithWarn returns the list of parameters for the files that should be tested including a file that should be warned.
func GetTestFileParamsWithWarn() []TestFileParams {
	return append(GetTestFileParams(), TestFileParams{
		TestName:      "Warned file",
		FileName:      "7ssns.txt",
		UlBlockLabel:  "",
		IsBad:         false,
		IsUnscannable: false,
		IsWarn:        true,
	})
}

// WaitForFCMTokenRegistered waits until a valid fcm token exists.
// This is done by downloading unknown_malware.zip from `download.html`.
// This function fails if scanning is disabled.
func WaitForFCMTokenRegistered(ctx context.Context, br *browser.Browser, tconnAsh *chrome.TestConn, server *httptest.Server, downloadsPath string) error {
	retryNumber := 0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		canRetry, err := checkFCMTokenRegistered(ctx, br, tconnAsh, server, downloadsPath, retryNumber)
		retryNumber++
		if canRetry {
			return err
		}
		return testing.PollBreak(err)
	}, &testing.PollOptions{Timeout: FcmTokenTimeOut, Interval: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for fcm token to be registered")
	}
	return nil
}

// checkFCMTokenRegistered checks that a fcm token is registered.
// Returns a bool and an error. The bool indicates whether the check can be retried.
func checkFCMTokenRegistered(ctx context.Context, br *browser.Browser, tconnAsh *chrome.TestConn, server *httptest.Server, downloadsPath string, retryNumber int) (bool, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dconnSafebrowsing, err := br.NewConn(ctx, "chrome://safe-browsing/#tab-deep-scan")
	if err != nil {
		return false, errors.Wrap(err, "failed to connect to chrome")
	}
	defer dconnSafebrowsing.Close()
	defer dconnSafebrowsing.CloseTarget(cleanupCtx)

	dconn, err := br.NewConn(ctx, server.URL+"/download.html")
	defer dconn.Close()
	defer dconn.CloseTarget(cleanupCtx)

	// Close all prior notifications.
	if err := ash.CloseNotifications(ctx, tconnAsh); err != nil {
		return false, errors.Wrap(err, "failed to close notifications")
	}

	if err := dconn.Eval(ctx, `document.getElementById("unknown_malware.zip").click()`, nil); err != nil {
		return false, errors.Wrap(err, "failed to click on link to download file")
	}

	// Check for notification (this might take some time in case of throttling).
	downloadBubbleState, err := WaitForDownloadViaDownloadBubble(ctx, tconnAsh, "unknown_malware.zip")
	if err != nil {
		return false, errors.Wrap(err, "failed to wait for download via download bubble UI")
	}

	if downloadBubbleState == DownloadBubbleStateUnavailable {
		if _, err := ash.WaitForNotification(
			ctx,
			tconnAsh,
			ScanningTimeOut,
			ash.WaitIDContains("notification-ui-manager"),
			ash.WaitTitleOrMessageContains("unknown_malware.zip"),
		); err != nil {
			return false, errors.Wrap(err, "failed to wait for notification")
		}
	}

	// Remove file if it was downloaded.
	defer os.Remove(filepath.Join(downloadsPath, "unknown_malware.zip"))

	// Check that scanning was at least initiated within 30s after download.
	// A row with a non-empty left column is added when scanning is initiated.
	// Once scanning is done, the right column is filled.
	// Note: This row is added only after fcm has connected, which can take up to 21s
	// (see CloudBinaryUploadService::RetryFCMConnection). So wait 30s for additional safety.
	var unusedVariable string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := dconnSafebrowsing.Eval(ctx, `(async () => {
			const table = document.getElementById("deep-scan-list");
			if (table.rows.length == 0) {
				// If there is no entry, scanning is not yet initiated.
				throw "Scanning not yet initiated.";
			}
			return ""
			})()`, &unusedVariable); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 2 * time.Second}); err != nil {
		testing.ContextLog(ctx, "scanning didn't initiate: ", err)
		// If scanning wasn't initiated within 30s, some initialization probably didn't happen yet.
		// Allow to retry it only twice, as the initialization shouldn't take very long.
		if retryNumber > 1 {
			return false, errors.Wrap(err, "scanning didn't initiate after the second retry")
		}
		return true, errors.Wrap(err, "scanning didn't initiate within 30s")
	}

	// Verify verdict.
	var failedToGetToken bool
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := dconnSafebrowsing.Eval(ctx, `(async () => {
			const table = document.getElementById("deep-scan-list");
			if (table.rows.length == 0) {
				// If there is no entry, scanning is not yet complete.
				throw "Scanning not yet complete, table empty";
			}
			// We check if the last entry is not empty to check whether there is an actual answer.
			innerHTML = table.rows[table.rows.length - 1].cells[1].innerHTML;
			if (innerHTML.includes("FAILED_TO_GET_TOKEN")) {
				return true;
			}
			if (innerHTML.includes("status")) {
				return false;
			}
			if (innerHTML.includes("UPLOAD_FAILURE")) {
				// Handle upload failure as token failure.
				return true;
			}
			throw "Scanning not yet complete";
			})()`, &failedToGetToken); err != nil {
			if err.Error() != "Scanning not yet complete" {
				testing.ContextLog(ctx, "Polling: ", err)
			}
			if strings.Contains(err.Error(), "rpcc: the connection is closing") {
				return testing.PollBreak(errors.Wrap(err, "Chrome has likely crashed"))
			}
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: ScanningTimeOut, Interval: 5 * time.Second}); err != nil {
		return false, errors.Wrap(err, "failed to wait for fcm token registration")
	}

	if failedToGetToken {
		// If FAILED_TO_GET_TOKEN is detected, we allow retries.
		testing.ContextLog(ctx, "FAILED_TO_GET_TOKEN detected")
		return true, errors.New("FAILED_TO_GET_TOKEN detected")
	}

	// Returning nil as error, will stop a poll.
	return true, nil
}

// WaitForDownloadViaDownloadBubble waits for a download via the download bubble UI.
// If the download bubble isn't enabled, it returns a state of UNAVAILABLE.
func WaitForDownloadViaDownloadBubble(ctx context.Context, tconnAsh *chrome.TestConn, filename string) (DownloadBubbleState, error) {
	ui := uiauto.New(tconnAsh)

	downloadsToolbarButton := nodewith.Role(role.Button).HasClass("DownloadToolbarButtonView")
	if err := ui.WithTimeout(2 * time.Second).WaitUntilExists(downloadsToolbarButton)(ctx); err != nil {
		testing.ContextLog(ctx, "Didn't show DownloadBubble: ", err)
		return DownloadBubbleStateUnavailable, nil
	}

	firstDownloadItem := nodewith.HasClass("DownloadBubbleRowView").First()

	if err := ui.LeftClickUntil(downloadsToolbarButton, ui.WithTimeout(time.Second).WaitUntilExists(firstDownloadItem))(ctx); err != nil {
		return "", errors.Wrap(err, "failed to click on downloadsToolbarButton")
	}

	firstDownloadFileLabel := nodewith.HasClass("Label").NameContaining(filename).Ancestor(firstDownloadItem)

	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(firstDownloadFileLabel)(ctx); err != nil {
		return "", errors.Wrap(err, "didn't show download item with correct file name")
	}

	firstDownloadStateLabelBlocked := nodewith.HasClass("Label").Ancestor(firstDownloadItem).NameContaining("Blocked")
	firstDownloadStateLabelAllowed := nodewith.HasClass("Label").Ancestor(firstDownloadItem).NameRegex(regexp.MustCompile("(Done|Scan is done)"))

	nodeFound, err := ui.WithTimeout(ScanningTimeOut).FindAnyExists(ctx, firstDownloadStateLabelBlocked, firstDownloadStateLabelAllowed)

	if err != nil {
		return "", errors.Wrap(err, "couldn't determine download bubble state")
	}

	if nodeFound == firstDownloadStateLabelBlocked {
		return DownloadBubbleStateBlocked, nil
	}

	return DownloadBubbleStateAllowed, nil
}

// WaitForDeepScanningVerdict waits until a valid deep scanning verdict is found.
func WaitForDeepScanningVerdict(ctx context.Context, dconnSafebrowsing *browser.Conn, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		var failureReason string
		if err := dconnSafebrowsing.Eval(ctx, `(async () => {
			const table = document.getElementById("deep-scan-list");
			if (table.rows.length == 0) {
				// If there is no entry, scanning is not yet complete.
				throw "Scanning not yet complete, table empty";
			}
			// We check if the last entry is not empty to check whether there is an actual answer.
			innerHTML = table.rows[table.rows.length - 1].cells[1].innerHTML;
			if (innerHTML.includes("status") || innerHTML.includes("FILE_ENCRYPTED")) {
				return "";
			}
			if (innerHTML.includes("TIMEOUT")) {
				return "TIMEOUT detected in row.";
			}
			if (innerHTML.includes("FAILED_TO_GET_TOKEN")) {
				return "FAILED_TO_GET_TOKEN detected in row.";
			}
			if (innerHTML.includes("UNKNOWN")) {
				return "UNKNOWN detected in row.";
			}
			// UNKNOWN is allowed to be present in the left column (as reason
			// when getting an fcm token), so we loop over the right column
			// (UNKNOWNs in the right column indicate an error).
			for (i=0; i<table.rows.length; i++) {
				if (table.rows[i].cells[1].innerHTML.includes("UNKNOWN")) {
					return "UNKNOWN detected in table.";
				}
			}
			throw "Scanning not yet complete";
			})()`, &failureReason); err != nil {
			if err.Error() != "Scanning not yet complete" {
				testing.ContextLog(ctx, "Polling: ", err)
			}
			if strings.Contains(err.Error(), "rpcc: the connection is closing") {
				return testing.PollBreak(errors.Wrap(err, "Chrome has likely crashed"))
			}
			return err
		}
		if failureReason != "" {
			// Failure reason is used for pollbreaks on unrecoverable errors.
			return testing.PollBreak(errors.New(failureReason))
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: 5 * time.Second})
}

// VerifyDeepScanningVerdict verifies that the deep scanning verdict corresponds to shouldBlock and shouldWarn.
func VerifyDeepScanningVerdict(ctx context.Context, dconnSafebrowsing *browser.Conn, shouldBlock, shouldWarn bool) error {
	data := make(map[string]bool)
	if err := dconnSafebrowsing.Eval(ctx, `(async () => {
		const table = document.getElementById("deep-scan-list");
		if (table.rows.length == 0) {
			// If there is no entry, scanning is not yet complete.
			throw 'No deep scanning verdict!';
		}
		if (table.rows[table.rows.length - 1].cells[1].innerHTML.length == 0) {
			throw 'Invalid empty response detected';
		}
		// We check if the last entry includes a block or warn message.
		const state = new Map();
		state["blocked"] = table.rows[table.rows.length - 1].cells[1].innerHTML.includes("BLOCK");
		state["warned"] = table.rows[table.rows.length - 1].cells[1].innerHTML.includes("WARN");
		return state;
		})()`, &data); err != nil {
		var tableHTML string
		if err := dconnSafebrowsing.Eval(ctx, `document.getElementById("deep-scan-list").outerHTML`, &tableHTML); err != nil {
			return errors.Wrap(err, "failed to get html of table")
		}
		return errors.Wrapf(err, "failed to check deep-scan-list entry. Html of table: %v", tableHTML)
	}
	if data["blocked"] != shouldBlock {
		var tableHTML string
		if err := dconnSafebrowsing.Eval(ctx, `document.getElementById("deep-scan-list").outerHTML`, &tableHTML); err != nil {
			return errors.Wrapf(err, "block state (%v) doesn't match expectation (%v). Failed to get html of table", data["blocked"], shouldBlock)
		}
		return errors.Errorf("block state (%v) doesn't match expectation (%v). Html of table: %v", data["blocked"], shouldBlock, tableHTML)
	}
	if data["warned"] != shouldWarn {
		var tableHTML string
		if err := dconnSafebrowsing.Eval(ctx, `document.getElementById("deep-scan-list").outerHTML`, &tableHTML); err != nil {
			return errors.Wrapf(err, "warn state (%v) doesn't match expectation (%v). Failed to get html of table", data["warned"], shouldWarn)
		}
		return errors.Errorf("warn state (%v) doesn't match expectation (%v). Html of table: %v", data["warned"], shouldWarn, tableHTML)
	}
	return nil
}

// GetCleanDconnSafebrowsing returns a Dconn to chrome://safe-browsing/#tab-deep-scan for which it is ensured that there is no prior deep scanning verdict.
func GetCleanDconnSafebrowsing(ctx context.Context, cr *chrome.Chrome, br *browser.Browser) (*browser.Conn, error) {
	var dconnSafebrowsing *browser.Conn
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
		defer cancel()

		var err error
		dconnSafebrowsing, err = br.NewConn(ctx, "chrome://safe-browsing/#tab-deep-scan")
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to connect to chrome"))
		}

		success := false
		defer func() {
			if success {
				return
			}
			if err := dconnSafebrowsing.CloseTarget(cleanupCtx); err != nil {
				testing.ContextLog(cleanupCtx, "Failed to close tab: ", err)
			}
			if err := dconnSafebrowsing.Close(); err != nil {
				testing.ContextLog(cleanupCtx, "Failed to close dconn: ", err)
			}
		}()

		var numRows int
		if err := dconnSafebrowsing.Eval(ctx, `document.getElementById("deep-scan-list").rows.length`, &numRows); err != nil {
			return testing.PollBreak(errors.Wrap(err, "could not verify numRows"))
		}
		if numRows != 0 {
			outputDir, ok := testing.ContextOutDir(ctx)
			if !ok {
				testing.ContextLog(ctx, "Couldn't get the output dir: ", err)
			} else {
				path := filepath.Join(outputDir, fmt.Sprintf("screenshot-clean-safe-browsing-page-verdict-exists.png"))
				if err := screenshot.CaptureChrome(ctx, cr, path); err != nil {
					testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
				}
			}
			return errors.Errorf("there already exists a deep scanning verdict, even though there shouldn't. numRows: %d", numRows)
		}
		success = true
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 10 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to wait for empty safe browsing site")
	}
	return dconnSafebrowsing, nil
}
