// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/defaultsearchprovider"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
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
			"dp-chromeos-eng@google.com",
			"anastasiian@chromium.org",
		},
		BugComponent: "b:1129862",
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

	// When searching for “abc” Annotation is recorded even when policy is
	// set to false, because a Url is loaded with http://abc
	// vs when policy is set to true/unset, URL loaded is
	// http://google.com/q=abc
	for _, param := range []defaultsearchprovider.TestCase{
		{
			Name:                 "enabled",
			Enabled:              true,
			Value:                &policy.DefaultSearchProviderEnabled{Val: true},
			ShouldFindAnnotation: true,
		},
		{
			Name:                 "disabled",
			Enabled:              false,
			Value:                &policy.DefaultSearchProviderEnabled{Val: false},
			ShouldFindAnnotation: true, // The Disabled value is not supported by the Google Admin console.
		},
		{
			Name:                 "unset",
			Enabled:              true,
			Value:                &policy.DefaultSearchProviderEnabled{Stat: policy.StatusUnset},
			ShouldFindAnnotation: true,
		},
	} {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{param.Value}); err != nil {
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

			if err := defaultsearchprovider.TriggerDefaultSearchProvider(ctx, param, tconn, br); err != nil {
				s.Fatal("Failed to trigger default search provider: ", err)
			}

			// Stop logging and check the logs for navigation_url_loader NetworkTrafficAnnotationTag.
			foundAnnotation, err := annotations.StopLoggingCheckLogs(ctx, cr, br, defaultsearchprovider.AnnotationID)
			if err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}

			if foundAnnotation && !param.ShouldFindAnnotation {
				s.Fatal("Unexpected NetworkTrafficAnnotationTag with id navigation_url_loader found")
			}

			if !foundAnnotation && param.ShouldFindAnnotation {
				s.Fatal("Failed to find NetworkTrafficAnnotationTag with id navigation_url_loader")
			}
		})
	}
}
