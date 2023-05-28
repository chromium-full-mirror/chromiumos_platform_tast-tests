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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BasicAuthOverHTTPEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that the BasicAuthOverHttpEnabled policy is properly applied",
		Contacts: []string{
			"imprivata-eng@google.com",
			"mpetrisor@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Identity > Imprivata
		BugComponent: "b:1253162",
		SoftwareDeps: []string{"chrome", "lacros"},
		Attr: []string{
			"group:golden_tier",
			"group:hw_agnostic",
		},
		Fixture: fixture.LacrosPolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.BasicAuthOverHttpEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func BasicAuthOverHTTPEnabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Create a server with basic auth.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Please enter your username and password for this site"`)
		// Always deny access, so that it requires auth and shows the authentication prompt window.
		w.WriteHeader(401)
		w.Write([]byte("Unauthorised.\n"))
	}))
	defer server.Close()

	for _, param := range []struct {
		name       string
		value      *policy.BasicAuthOverHttpEnabled
		wantPrompt bool
	}{
		{
			name:       "enabled",
			value:      &policy.BasicAuthOverHttpEnabled{Val: true},
			wantPrompt: true,
		},
		{
			name:       "disabled",
			value:      &policy.BasicAuthOverHttpEnabled{Val: false},
			wantPrompt: false,
		},
		{
			name:       "unset",
			value:      &policy.BasicAuthOverHttpEnabled{Stat: policy.StatusUnset},
			wantPrompt: true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeLacros)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			conn, err := br.NewConn(ctx, "")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}

			// Navigate to a page which requires authentication.
			if err := conn.Navigate(ctx, server.URL); err != nil {
				s.Fatalf("Failed to navigate to the server URL %q: %v", server.URL, err)
			}
			defer conn.Close()

			// Create a uiauto.Context with default timeout.
			ui := uiauto.New(tconn)

			promptWindow := nodewith.Name("Sign in").Role(role.Window)
			if param.wantPrompt {
				if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(promptWindow.First())(ctx); err != nil {
					s.Fatal("Failed to find the prompt window for authentication: ", err)
				}
			} else {
				if err := ui.EnsureGoneFor(promptWindow, 10*time.Second)(ctx); err != nil {
					s.Fatal("Failed to make sure no authentication prompt window popup: ", err)
				}
			}
		})
	}
}
