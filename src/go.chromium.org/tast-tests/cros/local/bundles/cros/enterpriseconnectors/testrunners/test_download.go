// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testrunners

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/helpers"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// TestDownload tests the behavior of enterprise connectors when downloading a file from a webpage.
// It checks:
// 1. Whether the file download is blocked or not
// 2. Whether the blocks notification appears when it should and vice versa or not
// 3. Whether the deep scan result is correct (especially relevant for AllowsImmediateDelivery==true)
func TestDownload(ctx context.Context, s *testing.State, cr *chrome.Chrome, cryptohomeUsername string) {
	// Verify policy.
	tconnAsh, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	devicePolicies, err := policyutil.PoliciesFromDUT(ctx, tconnAsh)
	if err != nil {
		s.Fatal("Failed to get device policies: ", err)
	}
	_, ok := devicePolicies.Chrome["OnFileDownloadedEnterpriseConnector"]
	testParams := s.Param().(helpers.TestParams)
	if !ok && testParams.ScansEnabled {
		s.Fatal("Policy isn't set, but should be")
	}
	if ok && !testParams.ScansEnabled {
		s.Fatal("Policy is set, but shouldn't be")
	}

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconnAsh); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconnAsh)

	dconn, err := cr.NewConn(ctx, "chrome://policy")
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer dconn.Close()
	defer dconn.CloseTarget(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error")

	// Clear Downloads directory.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cryptohomeUsername)
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	files, err := os.ReadDir(downloadsPath)
	if err != nil {
		s.Fatal("Failed to get files from Downloads directory")
	}
	for _, file := range files {
		if err = os.RemoveAll(filepath.Join(downloadsPath, file.Name())); err != nil {
			s.Fatal("Failed to remove file: ", file.Name())
		}
	}

	// Need to wait for a valid fcm token, i.e., the proper initialization of the enterprise connectors.
	if testParams.ScansEnabled {
		s.Log("Checking for fcm token")
		if err := helpers.WaitForFCMTokenRegistered(ctx, cr, tconnAsh, server, downloadsPath); err != nil {
			s.Fatal("Failed to wait for FCM token: ", err)
		}
	}

	for _, params := range helpers.GetTestFileParams() {
		if succeeded := s.Run(ctx, params.TestName, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			dconnSafebrowsing, err := helpers.GetCleanDconnSafebrowsing(ctx, cr)
			if err != nil {
				s.Fatal("Failed to get clean safe browsing page: ", err)
			}
			defer dconnSafebrowsing.Close()
			defer dconnSafebrowsing.CloseTarget(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error_safe_browsing_page")

			dlFileName := params.FileName

			shouldBlockDownload := false
			// For the report only UI (AllowsImmediateDelivery==true), no blocking should happen.
			if testParams.ScansEnabled && !testParams.AllowsImmediateDelivery {
				if params.IsUnscannable {
					shouldBlockDownload = !testParams.AllowsUnscannableFiles
				} else {
					shouldBlockDownload = params.IsBad
				}
			}

			dconn, err := cr.NewConn(ctx, server.URL+"/download.html")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}
			defer dconn.Close()
			defer dconn.CloseTarget(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error_file_download_page")

			// Close all prior notifications.
			if err := ash.CloseNotifications(ctx, tconnAsh); err != nil {
				s.Fatal("Failed to close notifications: ", err)
			}

			// The file name is also the ID of the link elements.
			if err := dconn.Eval(ctx, `document.getElementById('`+params.FileName+`').click()`, nil); err != nil {
				s.Fatal("Failed to execute JS expression: ", err)
			}

			// Cleanup file
			defer func() {
				if _, err := os.Stat(filepath.Join(downloadsPath, dlFileName)); !os.IsNotExist(err) {
					if err := os.Remove(filepath.Join(downloadsPath, dlFileName)); err != nil {
						s.Error("Failed to remove ", dlFileName, ": ", err)
					}
				}
			}()

			downloadBubbleState, err := helpers.WaitForDownloadViaDownloadBubble(ctx, tconnAsh, dlFileName)
			if err != nil {
				s.Fatal("Failed to wait for download via download bubble UI: ", err)
			}

			if downloadBubbleState == helpers.DownloadBubbleStateUnavailable {
				ntfctn, err := ash.WaitForNotification(
					ctx,
					tconnAsh,
					helpers.ScanningTimeOut,
					ash.WaitIDContains("notification-ui-manager"),
					ash.WaitTitleDoesntContain("Scanning"),
					ash.WaitTitleDoesntContain("Downloading"),
				)
				if err != nil {
					s.Fatal("Failed to wait for notification: ", err)
				}

				if shouldBlockDownload {
					if ntfctn.Title != "Dangerous download blocked" && !strings.Contains(ntfctn.Message, "blocked") {
						s.Fatal("Download should be blocked, but wasn't; notification: ", ntfctn)
					}
				} else {
					if ntfctn.Title != "Download complete" {
						s.Fatal("Download should be allowed, but wasn't; notification: ", ntfctn)
					}
				}
			} else {
				if shouldBlockDownload {
					if downloadBubbleState != helpers.DownloadBubbleStateBlocked {
						s.Fatal("Download should be blocked, but wasn't")
					}
				} else {
					if downloadBubbleState != helpers.DownloadBubbleStateAllowed {
						s.Fatal("Download should be allowed, but wasn't")
					}
				}
			}

			// Check file blocked/existence.
			_, err = os.Stat(filepath.Join(downloadsPath, dlFileName))
			if os.IsNotExist(err) {
				if !shouldBlockDownload {
					s.Error("Download was blocked, but shouldn't have been: ", err)
				}
			} else {
				if shouldBlockDownload {
					s.Error("Download was not blocked")
				}
			}

			if testParams.ScansEnabled {
				// If scans are enabled and the content isn't unscannable, we check the deep scanning verdict.
				if err := helpers.WaitForDeepScanningVerdict(ctx, dconnSafebrowsing, helpers.ScanningTimeOut); err != nil {
					s.Fatal("Failed to wait for deep scanning verdict: ", err)
				}
				if !params.IsUnscannable {
					if err := helpers.VerifyDeepScanningVerdict(ctx, dconnSafebrowsing, params.IsBad, params.IsWarn); err != nil {
						s.Fatal("Failed to verify deep scanning verdict: ", err)
					}
				}
			}
		}); !succeeded {
			// Stop, if the subtest fails as it might have left the state unusable.
			// It also prevents showing wrong errors on tastboard.
			break
		}
	}
}
