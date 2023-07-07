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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/advancedprotection"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AdvancedProtectionAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check behavior of the AdvancedProtectionAllowed policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr:         []string{"group:golden_tier"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"bad_app_file_on_scan.exe"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedInAdvancedProtection,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedInAdvancedProtection,
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AdvancedProtectionAllowed{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func AdvancedProtectionAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Setup local http server to serve a test suspicious file.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	for index, param := range advancedprotection.GetTestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
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
			netExport, err := netexport.Start(ctx, cr, br, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			// Trigger file upload for scanning. This should trigger the
			// safe_browsing_binary_upload_app annotation, if Advanced Protection is
			// enabled.
			if err := advancedprotection.TriggerUploadForScanning(ctx, nil, nil, br, server, tconn, index); err != nil {
				s.Fatal("Failed to trigger upload for scanning: ", err)
			}

			// Check the network logs to see if the file was uploaded for scanning.
			// Use polling since relying on the scan to complete successfully via the
			// UI has been flaky.
			foundAnnotation, err := netExport.FindUntil(ctx, advancedprotection.UploadAnnotationHashCode, &testing.PollOptions{
				Timeout:  15 * time.Second,
				Interval: 1 * time.Second,
			})
			if err != nil {
				s.Fatal("Failed to check logs: ", err)
			}

			if param.ShouldFindAnnotation != foundAnnotation {
				s.Fatalf("Unexpected state of safe_browsing_binary_upload_app annotation: got %t wanted %t", foundAnnotation, param.ShouldFindAnnotation)
			}
		})
	}
}
