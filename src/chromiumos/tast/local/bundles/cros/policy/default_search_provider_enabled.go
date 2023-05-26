// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/browser/browserui"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefaultSearchProviderEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of DefaultSearchProviderEnabled policy: check if a search provider is being automatically used",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"anastasiian@chromium.org",
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
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
			pci.SearchFlag(&policy.DefaultSearchProviderEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func DefaultSearchProviderEnabled(ctx context.Context, s *testing.State) {
	const (
		defaultSearchEngine = "google.com" // search engine checked in the test
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

	// Hash code for NetworkTrafficAnnotationTag with id navigation_url_loader.
	const navigationURLLoaderHashCode = "63171670"

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	for _, param := range []struct {
		name    string                               // name is the subtest name.
		enabled bool                                 // enabled is the expected enabled state of the policy.
		value   *policy.DefaultSearchProviderEnabled // value is the policy value.
		// shouldFindAnnotation states whether navigation_url_loader annotation should be found in the net-export log.
		shouldFindAnnotation bool
	}{
		{
			name:                 "true",
			enabled:              true,
			value:                &policy.DefaultSearchProviderEnabled{Val: true},
			shouldFindAnnotation: true,
		},
		{
			name:                 "false",
			enabled:              false,
			value:                &policy.DefaultSearchProviderEnabled{Val: false},
			shouldFindAnnotation: true, // The Disabled value is not supported by the Google Admin console.
		},
		{
			name:                 "unset",
			enabled:              true,
			value:                &policy.DefaultSearchProviderEnabled{Stat: policy.StatusUnset},
			shouldFindAnnotation: true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

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
			if err := kb.Type(ctx, "vy6ys\n"); err != nil {
				s.Fatal("Failed to write events: ", err)
			}

			// Wait for the page to load.
			if err := uiauto.WaitForLocation(addressBarNode)(ctx); err != nil {
				s.Fatal("Failed to wait for location change: ", err)
			}

			// Find the address bar.
			nodeInfo, err := uiauto.Info(ctx, addressBarNode)
			if err != nil {
				s.Fatal("Could not get new info for the address bar: ", err)
			}
			location := nodeInfo.Value

			defaultSearchEngineUsed := strings.Contains(location, defaultSearchEngine)
			if param.enabled != defaultSearchEngineUsed {
				s.Fatalf("Unexpected usage of search engine: got %t; want %t (got %q; want %q)", defaultSearchEngineUsed, param.enabled, location, defaultSearchEngine)
			}

			// Stop logging and check the logs for navigation_url_loader NetworkTrafficAnnotationTag.
			foundAnnotation, err := annotations.StopLoggingCheckLogs(ctx, cr, br, navigationURLLoaderHashCode)
			if err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}

			if foundAnnotation && !param.shouldFindAnnotation {
				s.Fatal("Unexpected NetworkTrafficAnnotationTag with id navigation_url_loader found")
			}

			if !foundAnnotation && param.shouldFindAnnotation {
				s.Fatal("Failed to find NetworkTrafficAnnotationTag with id navigation_url_loader")
			}
		})
	}
}
