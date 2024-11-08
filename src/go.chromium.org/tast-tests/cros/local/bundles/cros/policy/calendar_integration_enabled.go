// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/calendarintegration"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CalendarIntegrationEnabled,
		Desc:         "Checks behavior of CalendarIntegrationEnabled policy, check if event list is shown based on value of the policy",
		BugComponent: "b:1129862",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"crmullins@google.com",
		},
		Attr: []string{
			"group:golden_tier",
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		// TODO(b/302232315): mitmproxy fails to start on arm devices.
		// Remove architecture restrictions when the problem is solved.
		SoftwareDeps: []string{"chrome", "gaia", "amd64"},
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Fixture: fixture.ChromePolicyRealUserLoggedIn,
	})
}

func CalendarIntegrationEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

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

	for key, param := range calendarintegration.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr)
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			if err := calendarintegration.TriggerCalendarIntegration(ctx,
				networkrequestmonitor.OptionalServiceParams{
					Chrome:        cr,
					PolicySetting: key}); err != nil {
				s.Fatal("Failed to trigger and verify calendar integration: ", err)
			}

			// Check the net log for annotation.
			foundAnnotation, err := netExport.Find(calendarintegration.AnnotationHashCode)
			if err != nil {
				s.Fatal("Failed to check logs: ", err)
			}

			if param.ShouldFindAnnotation != foundAnnotation {
				s.Fatalf("Unexpected state of calendar_get_events annotation: got %t wanted %t", foundAnnotation, param.ShouldFindAnnotation)
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
