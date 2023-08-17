// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
	ukm "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/urlkeydatacollection"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const ukmTestURL = "https://www.google.com"

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrafficAnnotationURLKeyedDataCollection,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "This test checks the network annotation for UKM policy to make sure we are not sending network traffic when it's off",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"meyron@google.com",
			"rzakarian@google.com",
		},
		BugComponent: "b:1129862", // ChromeOS > Privacy > DPChromeOS > DPChromeOS Engineering
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
		Timeout:      8 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.ChromeEnrolledLoggedInShortMetricsInterval,
				Val:     browser.TypeAsh,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosEnrolledLoggedInShortMetricsInterval,
				Val:               browser.TypeLacros,
			},
		},
		Data: []string{"autofill_address_enabled.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UrlKeyedAnonymizedDataCollectionEnabled{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.EnableSyncConsent{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SyncDisabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func TrafficAnnotationURLKeyedDataCollection(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve 10 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	for index, param := range ukm.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.Policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br, false); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			if err := ukm.TriggerAndVerifyUkmAppFromIndex(ctx, br, index); err != nil {
				s.Fatal("Failed to verify log on ukm app: ", err)
			}

			hashCodes := []string{ukm.UkmNetworkAnnotationID}

			// wait to allow ukm time to write to log
			// (writes every 20 seconds, starting after 1 minute).
			hcLogStatus, err := annotations.PollMultipleAnnotation(ctx, cr, 80*time.Second, 10*time.Second, hashCodes)
			if err != nil {
				s.Fatal("Failed to poll hashcode in log: ", err)
			}

			// Stop logging.
			if err := annotations.StopLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}

			if hcLogStatus[ukm.UkmNetworkAnnotationID] != param.AnnotationLogExpected {
				s.Fatalf("Unexpected annotation; got: %t, want: %t", hcLogStatus[ukm.UkmNetworkAnnotationID],
					param.AnnotationLogExpected)
			}
		})
	}
}
