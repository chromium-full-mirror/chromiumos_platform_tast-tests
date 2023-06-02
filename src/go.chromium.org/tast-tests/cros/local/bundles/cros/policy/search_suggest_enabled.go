// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchSuggestEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of SearchSuggestEnabled policy, check if a search suggestions are shown based on the value of the policy",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"alexanderhartl@google.com", // Test author
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
			pci.SearchFlag(&policy.SearchSuggestEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func SearchSuggestEnabled(ctx context.Context, s *testing.State) {
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

	// Open a keyboard device.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open keyboard device: ", err)
	}
	defer keyboard.Close(ctx)

	// Hash code for NetworkTrafficAnnotationTag with id omnibox_suggest.
	const omniboxSuggestHashCode = "47815025"

	for _, param := range []struct {
		name    string
		enabled bool                         // enabled is the expected enabled state of the virtual keyboard.
		policy  *policy.SearchSuggestEnabled // policy is the policy we test.
		// shouldFindAnnotation states whether omnibox_suggest annotation should be found in the net-export log.
		shouldFindAnnotation bool
	}{
		{
			name:                 "unset",
			enabled:              true,
			policy:               &policy.SearchSuggestEnabled{Stat: policy.StatusUnset},
			shouldFindAnnotation: true,
		},
		{
			name:                 "disabled",
			enabled:              false,
			policy:               &policy.SearchSuggestEnabled{Val: false},
			shouldFindAnnotation: false,
		},
		{
			name:                 "enabled",
			enabled:              true,
			policy:               &policy.SearchSuggestEnabled{Val: true},
			shouldFindAnnotation: true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			conn, err := br.NewConn(ctx, "")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}
			defer conn.Close()

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			// Try to open a tab.
			if err := keyboard.Accel(ctx, "ctrl+t"); err != nil {
				s.Fatal("Failed to write events: ", err)
			}

			// Click the address bar.
			addressBar := browserui.AddressBarFinder
			ui := uiauto.New(tconn)
			if err := uiauto.Combine("find and click the address bar",
				ui.WaitUntilExists(addressBar),
				ui.LeftClick(addressBar),
			)(ctx); err != nil {
				s.Fatal("Failed to find and click the address bar: ", err)
			}

			// GoBigSleepLint - Wait for a second before typing to make sure the module for suggestions is loaded.
			testing.Sleep(ctx, time.Second)

			// Type something so suggestions pop up.
			if err := keyboard.Type(ctx, "google"); err != nil {
				s.Fatal("Failed to write events: ", err)
			}

			// Wait for the omnibox popup node.
			if err := ui.WaitUntilExists(nodewith.ClassName("OmniboxPopupViewViews"))(ctx); err != nil {
				s.Fatal("Failed to find omnibox popup: ", err)
			}

			// Get all the omnibox results.
			omniboxResults, err := ui.NodesInfo(ctx, nodewith.ClassName("OmniboxResultView"))
			if err != nil {
				s.Fatal("Failed to get omnibox results: ", err)
			}

			suggest := false
			for _, result := range omniboxResults {
				if strings.Contains(result.Name, "search suggestion") {
					suggest = true
					break
				}
			}

			if suggest != param.enabled {
				s.Errorf("Unexpected existence of search suggestions: got %t; want %t", suggest, param.enabled)
			}

			// Stop logging and check the logs for omnibox_suggest NetworkTrafficAnnotationTag.
			foundAnnotation, err := annotations.StopLoggingCheckLogs(ctx, cr, br, omniboxSuggestHashCode)
			if err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}

			if foundAnnotation && !param.shouldFindAnnotation {
				s.Fatal("Found unexpected NetworkTrafficAnnotationTag with id omnibox_suggest")
			}

			if !foundAnnotation && param.shouldFindAnnotation {
				s.Fatal("Did not find expected NetworkTrafficAnnotationTag with id omnibox_suggest")
			}
		})
	}
}
