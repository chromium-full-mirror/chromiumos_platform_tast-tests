// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/defaultsearchprovider"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DefaultSearchProviderEnabled,
		Desc: "Behavior of DefaultSearchProviderEnabled policy: check if a search provider is being automatically used",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"dp-chromeos-eng@google.com",
		},
		BugComponent: "b:1129862",
		// TODO(b/302232315): mitmproxy fails to start on arm devices.
		// Remove architecture restrictions when the problem is solved.
		SoftwareDeps: []string{"chrome", "no_arm"},
		Attr:         []string{"group:golden_tier", "group:mainline", "informational", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DefaultSearchProviderEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func DefaultSearchProviderEnabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	mp, err := proxy.NewMitmProxy(ctx,
		proxy.DumpHTTPFlow(true),
	)
	if err != nil {
		s.Fatal("Failed to start mitmproxy: ", err)
	}
	defer mp.Close(cleanupCtx)
	if err := mp.Connect(ctx, cr); err != nil {
		s.Fatal("Failed to configure chrome for proxy: ", err)
	}

	for key, param := range defaultsearchprovider.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr, br, browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			if err := defaultsearchprovider.TriggerDefaultSearchProvider(ctx,
				networkrequestmonitor.OptionalServiceParams{
					Chrome:        cr,
					Browser:       br,
					PolicySetting: key}); err != nil {
				s.Fatal("Failed to trigger default search provider: ", err)
			}

			// Check the logs for navigation_url_loader NetworkTrafficAnnotationTag.
			foundAnnotation, err := netExport.Find(defaultsearchprovider.AnnotationHashCode)
			if err != nil {
				s.Fatal("Failed to check net export logs: ", err)
			}

			if foundAnnotation && !param.ShouldFindAnnotation {
				s.Fatal("Unexpected NetworkTrafficAnnotationTag with id navigation_url_loader found")
			}

			if !foundAnnotation && param.ShouldFindAnnotation {
				s.Fatal("Failed to find NetworkTrafficAnnotationTag with id navigation_url_loader")
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
