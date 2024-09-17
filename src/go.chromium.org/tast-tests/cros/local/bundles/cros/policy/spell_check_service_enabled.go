// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/spellcheck"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SpellCheckServiceEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of SpellCheckServiceEnabled policy",
		Contacts: []string{
			"chrome-language@google.com",
			"dp-chromeos-eng@google.com",
			"megjablon@google.com",
		},
		BugComponent: "b:1457550",
		// TODO(b/302232315): mitmproxy fails to start on arm devices.
		// Remove architecture restrictions when the problem is solved.
		SoftwareDeps: []string{"chrome", "no_arm"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		Data: spellcheck.DataFiles(),
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SpellCheckServiceEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func SpellCheckServiceEnabled(ctx context.Context, s *testing.State) {
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

	// Setup and start webserver (implicitly provides data form above).
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	for key, param := range spellcheck.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup the browser for lacros tests after the policy was set.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr, br, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			if err := spellcheck.TriggerSpellCheck(ctx, networkrequestmonitor.OptionalServiceParams{
				Server:        server,
				Chrome:        cr,
				Browser:       br,
				PolicySetting: key}); err != nil {
				s.Fatal("Failed to trigger and verify spellcheck: ", err)
			}

			// Check the logs for annotation hashcode associated with the policy.
			foundAnnotation, err := netExport.Find(spellcheck.AnnotationHashCode)
			if err != nil {
				s.Fatal("Failed to search net log file for annotation: ", err)
			}
			if foundAnnotation != param.ShouldFindAnnotation {
				s.Fatalf("Annotation mismatch = got %t, want %t", foundAnnotation, param.ShouldFindAnnotation)
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
