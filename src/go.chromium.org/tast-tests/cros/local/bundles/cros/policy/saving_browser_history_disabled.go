// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SavingBrowserHistoryDisabled,
		Desc: "Behavior of SavingBrowserHistoryDisabled policy, check if browsing history entries are shown based on the value of the policy",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"alexanderhartl@google.com", // Test author
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SavingBrowserHistoryDisabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func SavingBrowserHistoryDisabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Create a server that serves an empty html document.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		io.WriteString(w, "")
	}))
	defer server.Close()

	for _, param := range []struct {
		name    string
		enabled bool                                 // enabled is the expected enabled state of the browsing history.
		policy  *policy.SavingBrowserHistoryDisabled // policy is the policy we test.
	}{
		{
			name:    "unset",
			enabled: true,
			policy:  &policy.SavingBrowserHistoryDisabled{Stat: policy.StatusUnset},
		},
		{
			name:    "history_disabled",
			enabled: false,
			policy:  &policy.SavingBrowserHistoryDisabled{Val: true},
		},
		{
			name:    "history_enabled",
			enabled: true,
			policy:  &policy.SavingBrowserHistoryDisabled{Val: false},
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

			// Clear the browser history.
			if err := tconn.Eval(ctx, `tast.promisify(chrome.browsingData.removeHistory({"since": 0}))`, nil); err != nil {
				s.Fatal("Failed to clear browsing history: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open a website to create a browsing history entry.
			conn, err := cr.NewConn(ctx, server.URL)
			if err != nil {
				s.Fatal("Failed to connect to the browser: ", err)
			}
			defer conn.Close()

			hconn, err := cr.NewConn(ctx, "chrome://history")
			if err != nil {
				s.Fatal("Failed to connect to the browser: ", err)
			}
			defer hconn.Close()

			ui := uiauto.New(tconn)

			// Check whether there is a browser history or not.
			histFound := false
			if err := testing.Poll(ctx, func(ctx context.Context) error {

				// TODO(b/203396229): Remove First() after fixing the duplication in the ui tree.
				// Check if there is a browser history entry.
				if exists, err := ui.IsNodeFound(ctx, nodewith.ClassName("website-link").Role(role.Link).First()); err != nil {
					return testing.PollBreak(errors.Wrap(err, "finding website-link node failed"))
				} else if exists {
					histFound = true
					return nil
				}

				// TODO(b/203396229): Remove First() after fixing the duplication in the ui tree.
				// Check if there is no browser history.
				if exists, err := ui.IsNodeFound(ctx, nodewith.Name("Your browsing history appears here").Role(role.StaticText).First()); err != nil {
					return testing.PollBreak(errors.Wrap(err, "finding text node failed"))
				} else if exists {
					histFound = false
					return nil
				}

				return errors.New("requested ui nodes not found")

			}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
				s.Fatal("Failed to check if history exists: ", err)
			}

			if histFound != param.enabled {
				s.Errorf("Unexpected existence of browser history found: got %t; want %t", histFound, param.enabled)
			}
		})
	}
}
