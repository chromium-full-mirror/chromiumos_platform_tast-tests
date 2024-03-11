// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefaultSearchProviderSearchURL,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of DefaultSearchProviderSearchURL policy: check if provided search provider is being used",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"dp-chromeos-eng@google.com",
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
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DefaultSearchProviderEnabled{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.DefaultSearchProviderSearchURL{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func DefaultSearchProviderSearchURL(ctx context.Context, s *testing.State) {
	const (
		fakeURL    = "fakeurl" // fakeURL is the fake search engine.
		searchTerm = "vy6ys"   // searchTerm is a value for test search.
	)
	addressBarNode := browserui.AddressBarFinder

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

	uiauto := uiauto.New(tconn)

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	for _, param := range []struct {
		name    string                                 // name is the subtest name.
		wantURL string                                 // wantURL is the expected search engine url.
		policy  *policy.DefaultSearchProviderSearchURL // policy is the value of DefaultSearchProviderSearchURL policy.
	}{
		{
			name:    "set",
			wantURL: fmt.Sprintf("%s/search?q=%s", fakeURL, searchTerm),
			// The URL should include the string '{searchTerms}', replaced in the query by the user's search terms.
			policy: &policy.DefaultSearchProviderSearchURL{Val: fmt.Sprintf("https://%s/search?q={searchTerms}", fakeURL)},
		},
		{
			name:    "unset",
			wantURL: "google.com",
			policy:  &policy.DefaultSearchProviderSearchURL{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			// DefaultSearchProviderSearchURL can specify the URL of the search engine only when DefaultSearchProviderEnabled is on.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{&policy.DefaultSearchProviderEnabled{Val: true}, param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Connect to Test API of the used browser to clear the browser
			// history. We need a second connection as the clearing of the
			// history has to be executed from the used browser while the
			// uiauto package needs a connection to the ash browser.
			tconn2, err := br.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to create Test API connection: ", err)
			}

			// Clear the browser history, otherwise the previous search results can
			// interfere with the test.
			if err := tconn2.Eval(ctx, `tast.promisify(chrome.browsingData.removeHistory({"since": 0}))`, nil); err != nil {
				s.Fatal("Failed to clear browsing history: ", err)
			}

			// Open an empty page.
			// Use chrome://newtab to open new tab page (see https://crbug.com/1188362#c19).
			conn, err := br.NewConn(ctx, "chrome://newtab/")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}
			defer conn.Close()

			// Click the address and search bar.
			if err := uiauto.LeftClick(addressBarNode)(ctx); err != nil {
				s.Fatal("Could not find the address bar: ", err)
			}

			// Type something.
			if err := kb.Type(ctx, searchTerm+"\n"); err != nil {
				s.Fatal("Failed to write events: ", err)
			}

			// Wait for the page to load.
			if err := uiauto.WaitForLocation(addressBarNode)(ctx); err != nil {
				s.Fatal("Failed to wait for location change: ", err)
			}

			nodeInfo, err := uiauto.Info(ctx, addressBarNode)
			if err != nil {
				s.Fatal("Could not get new info for the address bar: ", err)
			}
			location := nodeInfo.Value
			location = strings.TrimPrefix(location, "https://")
			location = strings.TrimPrefix(location, "www.")
			if !strings.HasPrefix(location, param.wantURL) {
				s.Fatalf("Unexpected search engine used: got %q; want %q", location, param.wantURL)
			}
		})
	}
}
