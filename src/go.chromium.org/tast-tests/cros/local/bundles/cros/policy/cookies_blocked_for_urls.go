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
		Func:         CookiesBlockedForUrls,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check that the CookiesBlockedForUrls policy blocks setting cookies on the given sites",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"nikitapodguzov@chromium.org", // Test author
		},
		BugComponent: "b:1263917",
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
		Data: []string{"cookies_test.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DefaultCookiesSetting{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.CookiesBlockedForUrls{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func CookiesBlockedForUrls(ctx context.Context, s *testing.State) {
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
		value *policy.CookiesBlockedForUrls
		allow bool
	}{
		{
			name:  "unset",
			value: &policy.CookiesBlockedForUrls{Stat: policy.StatusUnset},
			allow: true,
		},
		{
			name:  "block_single",
			value: &policy.CookiesBlockedForUrls{Val: []string{url}},
			allow: false,
		},
		{
			name:  "block_multiple",
			value: &policy.CookiesBlockedForUrls{Val: []string{"http://google.com", "http://doesnotmatter.com", url}},
			allow: false,
		},
		{
			name:  "allow_multiple",
			value: &policy.CookiesBlockedForUrls{Val: []string{"https://testingwebsite.html", "https://somewebsite.com", "http://doesnotmatter.com"}},
			allow: true,
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to reset Chrome: ", err)
			}

			// Set the default behavior to allow cookies.
			allowCookies := &policy.DefaultCookiesSetting{Val: 1}
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{tc.value, allowCookies}); err != nil {
				s.Fatal("Failed to serve and verify policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			conn, err := br.NewConn(ctx, url+"/cookies_test.html")
			if err != nil {
				s.Fatal("Failed to connect to Chrome: ", err)
			}
			defer conn.Close()

			allowed := false
			if err := conn.Eval(ctx, "cookiesAllowed", &allowed); err != nil {
				s.Fatal("Failed to read cookiesAllowed: ", err)
			}

			if allowed != tc.allow {
				s.Fatalf("Unexpected cookiesAllowed value; got %v, want %v", allowed, tc.allow)
			}
		})
	}
}
