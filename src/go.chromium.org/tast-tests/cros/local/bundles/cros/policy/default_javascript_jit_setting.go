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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefaultJavascriptJitSetting,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check that the DefaultJavaScriptJitSetting policy blocks or allows the JIT compiler",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"eariassoto@google.com", // Test author
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
		Data: []string{"jit_test.html", "is_jit_enabled.wasm"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DefaultJavaScriptJitSetting{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func DefaultJavascriptJitSetting(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	for _, tc := range []struct {
		name  string
		value *policy.DefaultJavaScriptJitSetting
		allow bool
	}{
		{
			name:  "allow",
			value: &policy.DefaultJavaScriptJitSetting{Val: 1},
			allow: true,
		},
		{
			name:  "block",
			value: &policy.DefaultJavaScriptJitSetting{Val: 2},
			allow: false,
		},
		{
			name:  "unset",
			value: &policy.DefaultJavaScriptJitSetting{Stat: policy.StatusUnset},
			allow: true,
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Error("Failed to reset Chrome: ", err)
			}

			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{tc.value}); err != nil {
				s.Error("Failed to serve and verify policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Error("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			conn, err := br.NewConn(ctx, server.URL+"/jit_test.html")
			if err != nil {
				s.Error("Failed to connect to Chrome: ", err)
			}
			defer conn.Close()

			jitAllowed := false
			if err := conn.Eval(ctx, "isJitEnabled()", &jitAllowed); err != nil {
				s.Error("Could not evaluate function isJitEnabled: ", err)
			}

			if jitAllowed != tc.allow {
				s.Errorf("Unexpected JIT compiler status: got %v, want %v", jitAllowed, tc.allow)
			}
		})
	}
}
