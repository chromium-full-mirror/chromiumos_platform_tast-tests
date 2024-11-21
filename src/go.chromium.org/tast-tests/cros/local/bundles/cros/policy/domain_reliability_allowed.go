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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/domainreliability"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DomainReliabilityAllowed,
		Desc: "This test checks for the network annotation for Domain Reliability to make sure we are not sending network traffic when the policy is disabled",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"alexwchen@google.com",
		},
		BugComponent: "b:1129862", // ChromeOS > Privacy > DPChromeOS > DPChromeOS Engineering
		SoftwareDeps: []string{"chrome", "amd64"},
		Attr:         []string{"group:golden_tier"},
		Data:         []string{"domain_reliability_500_requests.py"},
		Timeout:      10 * time.Minute,
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DomainReliabilityAllowed{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func DomainReliabilityAllowed(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	opts := []chrome.Option{
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	defer func(ctx context.Context) {
		// Use cr as a reference to close the last started Chrome instance.
		if err := cr.Close(ctx); err != nil {
			s.Error("Failed to close Chrome connection: ", err)
		}
	}(ctx)

	// Reserve 10 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	for key, param := range domainreliability.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Restart Chrome to trigger domain reliability setup.
			cr, err = chrome.New(ctx, opts...)
			if err != nil {
				s.Fatal("Chrome login failed: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr)
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			mp, err := proxy.NewMitmProxy(ctx,
				proxy.DumpHTTPFlow(true),
				proxy.ScriptPath(s.DataPath("domain_reliability_500_requests.py")),
			)
			if err != nil {
				s.Fatal("Failed to start mitmproxy: ", err)
			}
			defer mp.Close(cleanupCtx)
			if err := mp.Connect(ctx, cr); err != nil {
				s.Fatal("Failed to configure chrome for proxy: ", err)
			}

			if err := domainreliability.TriggerDomainReliabilityAllowed(ctx,
				networkrequestmonitor.OptionalServiceParams{
					Chrome:        cr,
					PolicySetting: key}); err != nil {
				s.Fatal("Failed to trigger and verify domain reliability: ", err)
			}

			// Check the logs for annotation domain_reliability_report_upload. Use polling
			// since the upload happens in the background and there is no UI notification.
			// Wait to allow time to write to log (starts after 1 minute).
			foundAnnotation, err := netExport.FindUntil(ctx, domainreliability.AnnotationHashCode, &testing.PollOptions{
				Timeout:  180 * time.Second,
				Interval: 10 * time.Second,
			})
			if err != nil {
				s.Fatal("Failed to check logs: ", err)
			}

			if param.ShouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Got: %t. Want: %t", foundAnnotation, param.ShouldFindAnnotation)
			}

			resp, err := mp.DumpHTTPFlow(ctx, true, true)
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
