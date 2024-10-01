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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	policyquickanswers "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/quickanswers"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: QuickAnswersUnitConversionEnabled,
		Desc: "Test QuickAnswersUnitConversionEnabled policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr: []string{
			"group:golden_tier",
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		// TODO(b/302232315): mitmproxy fails to start on arm devices.
		// Remove architecture restrictions when the problem is solved.
		SoftwareDeps: []string{"chrome", "amd64"},
		Data:         policyquickanswers.DataFiles(),
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Fixture: fixture.FakeDMSEnrolled,
	})
}

// QuickAnswersUnitConversionEnabled tests that Quick Answers unit conversion
// can be enabled and disabled via policy.
func QuickAnswersUnitConversionEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	mp, err := proxy.NewMitmProxy(ctx,
		proxy.CustomCA(true),
		proxy.DumpHTTPFlow(true),
	)
	if err != nil {
		s.Fatal("Failed to start mitmproxy: ", err)
	}
	defer mp.Close(cleanupCtx)

	opts := []chrome.Option{
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
		chrome.ProxyServer(mp.ProxyAddress()),
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := quickanswers.SetPrefValue(ctx, tconn, "settings.quick_answers.enabled", true); err != nil {
		s.Fatal("Failed to enable Quick Answers: ", err)
	}

	for key, param := range policyquickanswers.UnitConversionTestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup a browser.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr, br, browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			if err := policyquickanswers.TriggerQuickAnswersUnitConversion(ctx,
				networkrequestmonitor.OptionalServiceParams{
					Chrome:        cr,
					Browser:       br,
					Server:        server,
					PolicySetting: key}); err != nil {
				s.Fatal("Failed to trigger and verify quick answers unit conversion: ", err)
			}

			// Check the logs for annotation hashcode associated with the policy.
			foundAnnotation, err := netExport.Find(policyquickanswers.AnnotationHashCode)
			if err != nil {
				s.Fatal("Failed to check net export logs: ", err)
			}
			if param.ShouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Expected: %t. Actual: %t", param.ShouldFindAnnotation, foundAnnotation)
			}

			resp, err := mp.DumpHTTPFlow(ctx, true, false)
			if err != nil {
				s.Fatal("Failed to get dump httpflow from mitmproxy: ", err)
			}

			v := proxy.NewNetworkVerifier(resp)
			allow := param.TrafficShouldFind
			disallow := param.TrafficShouldNotFind
			// allow/disallow traffic are passed by test case param.
			// We will ignore traffic other than the allow and disallow list for verifying diff.
			if err := v.Verify(allow, disallow, []string{".*"}); err != nil {
				s.Fatal("Diff test result: ", err)
			}
		})
	}
}
