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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    TestFileAttachedUserSession,
		Desc:    "Enterprise connector test for uploading files in user sessions",
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
			"group:golden_tier_secondary",
			"group:medium_low_tier",
			"group:hardware",
		},
		Params: []testing.Param{
			{
				Name:    "scan_enabled_allows_immediate_and_unscannable_ash",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledAllowExtra",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            true,
				},
			},
			{
				Name:    "scan_enabled_blocks_immediate_and_unscannable_ash",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledBlockExtra",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: false,
					AllowsUnscannableFiles:  false,
					ScansEnabled:            true,
				},
			},
			{
				Name:    "scan_disabled_ash",
				Fixture: "ashGaiaSignedInProdPolicyWPDisabled",
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

func TestFileAttachedUserSession(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	testrunners.TestFileAttached(ctx, s, cr, cr.NormalizedUser())
}
