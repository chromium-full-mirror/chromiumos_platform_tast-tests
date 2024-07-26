// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/helpers"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/testrunners"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TestFileTransferMGS,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Enterprise connector test for transferring files between different file systems on Managed Guest Sessions",
		Timeout:      30 * time.Minute,
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"muhamedp@google.com",
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
				Fixture: fixture.EnterpriseConnectorsMGSAshWebProtectEnabledAllowEnrolled,
				Val: helpers.FileTransferTestParams{
					TestParams: helpers.TestParams{
						AllowsImmediateDelivery: true,
						AllowsUnscannableFiles:  true,
						ScansEnabled:            true,
						BrowserType:             browser.TypeAsh,
					},
					// On Managed Guest Sessions, we don't get a mounted Drive directory, so we only test USB file transfers
					FileSystem: helpers.FileSystemTypeUSB,
				},
			},
			{
				Name:    "scan_enabled_blocks_immediate_and_unscannable",
				Fixture: fixture.EnterpriseConnectorsMGSAshWebProtectEnabledBlockEnrolled,
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
				Name:    "scan_disabled",
				Fixture: fixture.EnterpriseConnectorsMGSAshWebProtectDisabledEnrolled,
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

func TestFileTransferMGS(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx, chrome.KeepEnrollment(), chrome.NoLogin())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	testrunners.TestFileTransfer(ctx, s, cr, helpers.FakeMgsUsername)
}
