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
		Func:         JavaScriptBlockedForUrls,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check that the JavaScriptBlockedForUrls policy blocks execution of JavaScript on the given sites",
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
			pci.SearchFlag(&policy.DefaultJavaScriptSetting{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.JavaScriptBlockedForUrls{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func JavaScriptBlockedForUrls(ctx context.Context, s *testing.State) {
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
		value *policy.JavaScriptBlockedForUrls
		allow bool
	}{
		{
			name:  "unset",
			value: &policy.JavaScriptBlockedForUrls{Stat: policy.StatusUnset},
			allow: true,
		},
		{
			name:  "block_single",
			value: &policy.JavaScriptBlockedForUrls{Val: []string{url + "/js_test.hmtl"}},
			allow: false,
		},
		{
			name:  "allow_multiple",
			value: &policy.JavaScriptBlockedForUrls{Val: []string{"http://www.bing.com", "https://www.yahoo.com"}},
			allow: true,
		},
		{
			name:  "block_multiple",
			value: &policy.JavaScriptBlockedForUrls{Val: []string{"http://www.bing.com", "https://www.yahoo.com", url}},
			allow: false,
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to reset Chrome: ", err)
			}

			// Set the default behavior to allow JavaScript.
			allowJavaScript := &policy.DefaultJavaScriptSetting{Val: 1}
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{tc.value, allowJavaScript}); err != nil {
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
