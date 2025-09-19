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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    TestFileAttachedMGS,
		Desc:    "Enterprise connector test for uploading files in MGS",
		Timeout: 30 * time.Minute,
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
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier_secondary",
			"group:medium_low_tier",
			"group:hardware",
		},
		Params: []testing.Param{
			{
				Name:    "scan_enabled_allows_immediate_and_unscannable",
				Fixture: fixture.EnterpriseConnectorsMGSAshWebProtectEnabledAllowEnrolled,
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            true,
				},
			},
			{
				Name:    "scan_enabled_blocks_immediate_and_unscannable",
				Fixture: fixture.EnterpriseConnectorsMGSAshWebProtectEnabledBlockEnrolled,
				Val: helpers.TestParams{
					AllowsImmediateDelivery: false,
					AllowsUnscannableFiles:  false,
					ScansEnabled:            true,
				},
			},
			{
				Name:    "scan_disabled",
				Fixture: fixture.EnterpriseConnectorsMGSAshWebProtectDisabledEnrolled,
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            false,
				},
			},
		},
		Data: []string{
			"download.html", // download.html required for CheckFCMTokenRegistered.
			"file_input.html",
			"10ssns.txt",
			"allowed.txt",
			"content.exe",
			"unknown_malware_encrypted.zip",
			"unknown_malware.zip",
		},
	})
}

func TestFileAttachedMGS(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx, chrome.KeepEnrollment(), chrome.NoLogin())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	testrunners.TestFileAttached(ctx, s, cr, helpers.FakeMgsUsername)
}
