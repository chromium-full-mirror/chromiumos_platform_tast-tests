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
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/spellcheck"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/proxy/mitmproxy"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"

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
			"megjablon@google.com",
		},
		BugComponent: "crbug:UI>Browser>Language>Spellcheck",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
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
		Data: spellcheck.GetDataFiles(),
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

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
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
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Value}); err != nil {
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
			if err := annotations.StartLogging(ctx, cr, br, false); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			mp := mitmproxy.New()
			mp.SetBinaryPath(s.DataPath(mitmproxy.MitmdumpBinFile)).SetOutDir(s.OutDir())
			cleanup, err := cr.LaunchAndApplyProxy(ctx, mp)
			if err != nil {
				s.Fatal("Failed to launch and apply proxy: ", err)
			}
			defer cleanup(cleanupCtx)

			if err := spellcheck.TriggerSpellCheck(ctx, cr, br, server, tconn, key); err != nil {
				s.Fatal("Failed to trigger and verify spellcheck: ", err)
			}

			// Stop logging and check the logs for annotation hashcode associated with
			// the policy.
			foundAnnotation, err := annotations.StopLoggingCheckLogs(ctx, cr, br, spellcheck.AnnotationHashCode)
			if err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}

			if foundAnnotation && !param.ShouldFindAnnotation {
				s.Fatal("Found unexpected NetworkTrafficAnnotationTag with id spellcheck_lookup")
			}

			if !foundAnnotation && param.ShouldFindAnnotation {
				s.Fatal("Did not find expected NetworkTrafficAnnotationTag with id spellcheck_lookup")
			}
		})
	}
}
