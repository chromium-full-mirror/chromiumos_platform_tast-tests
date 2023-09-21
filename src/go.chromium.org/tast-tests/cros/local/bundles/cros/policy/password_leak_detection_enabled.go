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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/passwordleakdetection"
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
		Func:         PasswordLeakDetectionEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test PasswordLeakDetectionEnabled policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr: []string{
			"group:golden_tier",
			"group:mainline",
			"informational",
			"group:criticalstaging",
		},
		Data:         passwordleakdetection.GetDataFiles(),
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PasswordLeakDetectionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SafeBrowsingProtectionLevel{}, pci.VerifiedValue),
		},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyRealUserLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           fixture.LacrosPolicyRealUserLoggedIn,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// PasswordLeakDetectionEnabled verifies that the PasswordLeakDetectionEnabled
// policy disables the `lookup_single_password_leak` network call.
func PasswordLeakDetectionEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	for _, param := range passwordleakdetection.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			policies := []policy.Policy{
				param.Policy,
				// Enable Safe Browsing, which is required for password leak detection.
				&policy.SafeBrowsingProtectionLevel{Val: 1},
			}
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup a browser.
			bt := s.Param().(browser.Type)
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr, br, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			if err := passwordleakdetection.TriggerPasswordLeakDetection(ctx, cr, br, server, nil, 0); err != nil {
				s.Fatal("Failed to trigger password leak detection: ", err)
			}

			// Check the logs for annotation lookup_single_password_leak. Use polling
			// since the password check happens in the background, and there may not
			// always be a UI popup to notify that it is done.
			foundAnnotation, err := netExport.FindUntil(ctx, passwordleakdetection.AnnotationHashCode,
				&testing.PollOptions{Timeout: 5 * time.Second, Interval: 1 * time.Second})
			if err != nil {
				s.Fatal("Failed to check network logs: ", err)
			}

			// Verify network annotation is present when expected.
			if param.ShouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Expected: %t. Actual: %t", param.ShouldFindAnnotation, foundAnnotation)
			}
		})
	}
}
