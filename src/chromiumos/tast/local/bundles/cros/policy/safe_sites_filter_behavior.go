// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/policyutil"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const unsafeSite = "https://porn.com"

func init() {
	testing.AddTest(&testing.Test{
		Func:         SafeSitesFilterBehavior,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check that the SafeSitesFilterBehavior policy can block/allow unsafe sites",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"jeroendh@google.com", // Test author
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
			pci.SearchFlag(&policy.SafeSitesFilterBehavior{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func SafeSitesFilterBehavior(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	for _, tc := range []struct {
		name            string
		value           *policy.SafeSitesFilterBehavior
		shouldBeBlocked bool
	}{
		{
			name:            "enabled",
			value:           &policy.SafeSitesFilterBehavior{Val: 1},
			shouldBeBlocked: true,
		},
		{
			name:            "disabled",
			value:           &policy.SafeSitesFilterBehavior{Val: 0},
			shouldBeBlocked: false,
		},
		{
			name:            "unset",
			value:           &policy.SafeSitesFilterBehavior{Stat: policy.StatusUnset},
			shouldBeBlocked: false,
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to reset Chrome: ", err)
			}

			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{tc.value}); err != nil {
				s.Fatal("Failed to serve and verify policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			conn, err := br.NewConn(ctx, unsafeSite)
			if err != nil {
				s.Fatal("Failed to connect to Chrome: ", err)
			}
			defer conn.Close()

			content, err := conn.PageContent(ctx)
			if err != nil {
				s.Fatal("Failed to access the page content: ", err)
			}

			isBlocked := strings.Contains(content, "ERR_BLOCKED_BY_ADMINISTRATOR")

			if isBlocked != tc.shouldBeBlocked {
				if tc.shouldBeBlocked {
					s.Error("Unsafe content should be blocked, but it is not blocked")
				} else {
					s.Error("Unsafe content should not be blocked, but it is blocked")
				}
			}
		})
	}
}
