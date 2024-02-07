// Copyright 2020 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         JavaScriptAllowedForUrls,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check that the JavaScriptAllowedForUrls policy allows execution of JavaScript only on the given sites",
		Contacts: []string{
			"chrome-permissions-team@google.com",
		},
		BugComponent: "b:1456817",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		Data: []string{"js_test.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.JavaScriptAllowedForUrls{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.DefaultJavaScriptSetting{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func JavaScriptAllowedForUrls(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	url := server.URL

	for _, tc := range []struct {
		name  string
		value *policy.JavaScriptAllowedForUrls
		allow bool
	}{
		{
			name:  "unset",
			value: &policy.JavaScriptAllowedForUrls{Stat: policy.StatusUnset},
			allow: false,
		},
		{
			name:  "allow_single",
			value: &policy.JavaScriptAllowedForUrls{Val: []string{url + "/js_test.html"}},
			allow: true,
		},
		{
			name:  "allow_multiple",
			value: &policy.JavaScriptAllowedForUrls{Val: []string{"http://www.bing.com", "https://www.yahoo.com", url}},
			allow: true,
		},
		{
			name:  "block_multiple",
			value: &policy.JavaScriptAllowedForUrls{Val: []string{"http://www.bing.com", "https://www.yahoo.com"}},
			allow: false,
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to reset Chrome: ", err)
			}

			// For sites not included in allowlist Chrome falls back to default policy.
			blockJavaScript := &policy.DefaultJavaScriptSetting{Val: 2}
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{tc.value, blockJavaScript}); err != nil {
				s.Fatal("Failed to serve and verify policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			conn, err := br.NewConn(ctx, url+"/js_test.html")
			if err != nil {
				s.Fatal("Failed to connect to Chrome: ", err)
			}
			defer conn.Close()

			allowed := false
			// Evaluating is going to fail if JavaScript is not allowed.
			if err := conn.Eval(ctx, "jsAllowed", &allowed); err != nil && tc.allow {
				s.Fatal("Failed to read jsAllowed although JavaScript should be allowed: ", err)
			} else {
				if allowed != tc.allow {
					s.Fatalf("Unexpected jsAllowed value; got %v, want %v", allowed, tc.allow)
				}
			}
		})
	}
}
