// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/helpers"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/testrunners"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    TestFileTransferUserSession,
		Desc:    "Enterprise connector test for transferring files between different file systems",
		Timeout: 30 * time.Minute,
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"sseckler@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"gaia",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
		},
		Params: []testing.Param{
			{
				Name:    "scan_enabled_allows_immediate_and_unscannable",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledAllowExtra",
				Val: helpers.FileTransferTestParams{
					TestParams: helpers.TestParams{
						AllowsImmediateDelivery: true,
						AllowsUnscannableFiles:  true,
						ScansEnabled:            true,
						BrowserType:             browser.TypeAsh,
					},
					FileSystem: helpers.FileSystemTypeUSB,
				},
			},
			{
				Name:    "scan_enabled_allows_immediate_and_unscannable_drive",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledAllowExtra",
				Val: helpers.FileTransferTestParams{
					TestParams: helpers.TestParams{
						AllowsImmediateDelivery: true,
						AllowsUnscannableFiles:  true,
						ScansEnabled:            true,
						BrowserType:             browser.TypeAsh,
					},
					FileSystem: helpers.FileSystemTypeGDrive,
				},
			},
			{
				Name:    "scan_enabled_blocks_immediate_and_unscannable",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledBlockExtra",
				Val: helpers.FileTransferTestParams{
					TestParams: helpers.TestParams{
						AllowsImmediateDelivery: false,
						AllowsUnscannableFiles:  false,
						ScansEnabled:            true,
						BrowserType:             browser.TypeAsh,
					},
					FileSystem: helpers.FileSystemTypeUSB,
				},
			},
			{
				Name:    "scan_enabled_blocks_immediate_and_unscannable_drive",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledBlockExtra",
				Val: helpers.FileTransferTestParams{
					TestParams: helpers.TestParams{
						AllowsImmediateDelivery: false,
						AllowsUnscannableFiles:  false,
						ScansEnabled:            true,
						BrowserType:             browser.TypeAsh,
					},
					FileSystem: helpers.FileSystemTypeGDrive,
				},
			},
			{
				Name:    "scan_disabled",
				Fixture: "ashGaiaSignedInProdPolicyWPDisabled",
				Val: helpers.FileTransferTestParams{
					TestParams: helpers.TestParams{
						AllowsImmediateDelivery: true,
						AllowsUnscannableFiles:  true,
						ScansEnabled:            false,
						BrowserType:             browser.TypeAsh,
					},
					FileSystem: helpers.FileSystemTypeUSB,
				},
			},
		},
		Data: []string{
			"download.html", // download.html required for CheckFCMTokenRegistered.
			"7ssns.txt",
			"10ssns.txt",
			"allowed.txt",
			"content.exe",
			"unknown_malware_encrypted.zip",
			"unknown_malware.zip",
		},
	})
}

func TestFileTransferUserSession(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	testrunners.TestFileTransfer(ctx, s, cr, cr.NormalizedUser())
}
