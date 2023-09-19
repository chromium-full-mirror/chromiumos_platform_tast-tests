// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/helpers"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TestDownload,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Enterprise connector test for downloading files",
		Timeout:      30 * time.Minute,
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"sseckler@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Params: []testing.Param{
			{
				Name:    "scan_enabled_allows_immediate_and_unscannable_ash",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledAllowExtra",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            true,
					BrowserType:             browser.TypeAsh,
				},
			},
			{
				Name:    "scan_enabled_blocks_immediate_and_unscannable_ash",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledBlockExtra",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: false,
					AllowsUnscannableFiles:  false,
					ScansEnabled:            true,
					BrowserType:             browser.TypeAsh,
				},
			},
			{
				Name:    "scan_disabled_ash",
				Fixture: "ashGaiaSignedInProdPolicyWPDisabled",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            false,
					BrowserType:             browser.TypeAsh,
				},
			},
			{
				Name:    "scan_enabled_allows_immediate_and_unscannable_lacros",
				Fixture: "lacrosGaiaSignedInProdPolicyWPEnabledAllowExtra",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            true,
					BrowserType:             browser.TypeLacros,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "scan_enabled_blocks_immediate_and_unscannable_lacros",
				Fixture: "lacrosGaiaSignedInProdPolicyWPEnabledBlockExtra",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: false,
					AllowsUnscannableFiles:  false,
					ScansEnabled:            true,
					BrowserType:             browser.TypeLacros,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "scan_disabled_lacros",
				Fixture: "lacrosGaiaSignedInProdPolicyWPDisabled",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            false,
					BrowserType:             browser.TypeLacros,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
		Data: []string{
			"download.html",
			"10ssns.txt",
			"allowed.txt",
			"content.exe",
			"unknown_malware_encrypted.zip",
			"unknown_malware.zip",
		},
	})
}

func TestDownload(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Clear Downloads directory.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	files, err := ioutil.ReadDir(downloadsPath)
	if err != nil {
		s.Fatal("Failed to get files from Downloads directory")
	}
	for _, file := range files {
		if err = os.RemoveAll(filepath.Join(downloadsPath, file.Name())); err != nil {
			s.Fatal("Failed to remove file: ", file.Name())
		}
	}

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

	// Create Browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, testParams.BrowserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	tconnBrowser, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser's test API: ", err)
	}

	// The browsers sometimes restore some tabs, so we manually close all unneeded tabs.
	closeTabsFunc := browser.CloseAllTabs
	if testParams.BrowserType == browser.TypeLacros {
		// For lacros-Chrome, it should leave a new tab to keep the Chrome process alive.
		closeTabsFunc = browser.ReplaceAllTabsWithSingleNewTab
	}
	if err := closeTabsFunc(ctx, tconnBrowser); err != nil {
		s.Fatal("Failed to close all unneeded tabs: ", err)
	}
	defer closeTabsFunc(cleanupCtx, tconnBrowser)

	dconn, err := br.NewConn(ctx, "chrome://policy")
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer dconn.Close()
	defer dconn.CloseTarget(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error")

	// Need to wait for a valid fcm token, i.e., the proper initialization of the enterprise connectors.
	if testParams.ScansEnabled {
		s.Log("Checking for fcm token")
		if err := helpers.WaitForFCMTokenRegistered(ctx, br, tconnAsh, server, downloadsPath); err != nil {
			s.Fatal("Failed to wait for FCM token: ", err)
		}
	}

	for _, params := range helpers.GetTestFileParams() {
		if succeeded := s.Run(ctx, params.TestName, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			cr := s.FixtValue().(chrome.HasChrome).Chrome()
			dconnSafebrowsing, err := helpers.GetCleanDconnSafebrowsing(ctx, cr, br)
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

			dconn, err := br.NewConn(ctx, server.URL+"/download.html")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}
			defer dconn.Close()
			defer dconn.CloseTarget(cleanupCtx)

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
