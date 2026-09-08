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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type blocklistTestTable struct {
	name        string          // name is the subtest name.
	blockedURLs []string        // blockedURLs is a list of urls expected to be blocked.
	allowedURLs []string        // allowedURLs is a list of urls expected to be accessible.
	policies    []policy.Policy // policies is a list of URLBlocklist, URLAllowlist, URLBlacklist and URLWhitelist policies to update before checking urls.
}

func init() {
	testing.AddTest(&testing.Test{
		Func: URLCheck,
		Desc: "Checks the behavior of URL allow/deny-listing policies",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"vsavu@google.com", // Test author
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Params: []testing.Param{
			{
				Name:    "blocklist",
				Fixture: fixture.ChromePolicyLoggedIn,
				Val: []blocklistTestTable{
					{
						name:        "single",
						blockedURLs: []string{"http://example.org/blocked.html"},
						allowedURLs: []string{"http://google.com", "http://chromium.org"},
						policies:    []policy.Policy{&policy.URLBlocklist{Val: []string{"http://example.org/blocked.html"}}},
					},
					{
						name:        "multi",
						blockedURLs: []string{"http://example.org/blocked1.html", "http://example.org/blocked2.html"},
						allowedURLs: []string{"http://google.com", "http://chromium.org"},
						policies:    []policy.Policy{&policy.URLBlocklist{Val: []string{"http://example.org/blocked1.html", "http://example.org/blocked2.html"}}},
					},
					{
						name:        "wildcard",
						blockedURLs: []string{"http://example.com/blocked1.html", "http://example.com/blocked2.html"},
						allowedURLs: []string{"http://google.com", "http://chromium.org"},
						policies:    []policy.Policy{&policy.URLBlocklist{Val: []string{"example.com"}}},
					},
					{
						name:        "chrome-policy",
						blockedURLs: []string{"chrome://policy"},
						allowedURLs: []string{"http://google.com", "http://chromium.org", "chrome://about"},
						policies:    []policy.Policy{&policy.URLBlocklist{Val: []string{"chrome://policy"}}},
					},
					{
						name:        "wildcard-chrome",
						blockedURLs: []string{"chrome://about", "chrome://policy"},
						allowedURLs: []string{"http://google.com", "http://chromium.org"},
						policies:    []policy.Policy{&policy.URLBlocklist{Val: []string{"chrome://*"}}},
					},
					{
						name:        "unset",
						blockedURLs: []string{},
						allowedURLs: []string{"http://google.com", "http://chromium.org"},
						policies:    []policy.Policy{&policy.URLBlocklist{Stat: policy.StatusUnset}},
					},
				},
			},
			{
				Name:    "allowlist",
				Fixture: fixture.ChromePolicyLoggedIn,
				Val: []blocklistTestTable{
					{
						name:        "single",
						blockedURLs: []string{"http://example.org"},
						allowedURLs: []string{"http://chromium.org"},
						policies: []policy.Policy{
							&policy.URLBlocklist{Val: []string{"org"}},
							&policy.URLAllowlist{Val: []string{"chromium.org"}},
						},
					},
					{
						name:        "identical",
						blockedURLs: []string{"http://example.org"},
						allowedURLs: []string{"http://chromium.org"},
						policies: []policy.Policy{
							&policy.URLBlocklist{Val: []string{"http://chromium.org", "http://example.org"}},
							&policy.URLAllowlist{Val: []string{"http://chromium.org"}},
						},
					},
					{
						name:        "https",
						blockedURLs: []string{"http://chromium.org"},
						allowedURLs: []string{"https://chromium.org"},
						policies: []policy.Policy{
							&policy.URLBlocklist{Val: []string{"chromium.org"}},
							&policy.URLAllowlist{Val: []string{"https://chromium.org"}},
						},
					},
					{
						name:        "unset",
						blockedURLs: []string{},
						allowedURLs: []string{"http://chromium.org"},
						policies: []policy.Policy{
							&policy.URLBlocklist{Stat: policy.StatusUnset},
							&policy.URLAllowlist{Stat: policy.StatusUnset},
						},
					},
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.URLBlocklist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.URLAllowlist{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func URLCheck(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tcs, ok := s.Param().([]blocklistTestTable)
	if !ok {
		s.Fatal("Failed to convert test cases to the desired type")
	}

	for _, tc := range tcs {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, tc.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Run actual test.
			urlBlocked := func(url string) (bool, string) {
				conn, err := cr.NewConn(ctx, url)
				if err != nil {
					s.Fatal("Failed to connect to chrome: ", err)
				}
				defer conn.Close()
				defer conn.CloseTarget(ctx)

				var message string
				err = testing.Poll(ctx, func(ctx context.Context) error {
					if err := conn.Eval(ctx, `document.getElementById("main-message").innerText`, &message); err != nil {
						return err
					}
					if strings.Contains(message, "ERR_BLOCKED_BY_ADMINISTRATOR") || strings.Contains(message, "blocked") {
						return nil
					}
					return errors.New("error message not rendered yet")
				}, &testing.PollOptions{Timeout: 5 * time.Second})

				return err == nil, message
			}

			for _, allowed := range tc.allowedURLs {
				blocked, _ := urlBlocked(allowed)
				if blocked {
					s.Errorf("Expected %q to load", allowed)
				}
			}

			for _, blocked := range tc.blockedURLs {
				blockedResult, message := urlBlocked(blocked)
				if !blockedResult {
					if len(message) > 100 {
						message = message[:100] + "..."
					}
					s.Errorf("Expected %q to be blocked, but was not. Page content snippet: %q", blocked, message)
				}
			}
		})
	}
}
