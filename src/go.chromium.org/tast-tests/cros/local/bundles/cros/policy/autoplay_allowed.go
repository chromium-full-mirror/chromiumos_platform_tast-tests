// Copyright 2021 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var errNotPlaying = errors.New("media is not playing")

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutoplayAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checking if autoplay is allowed on websites or not, depending on the value of the policy",
		Contacts: []string{
			"chrome-media-ux@google.com",
			"alexanderhartl@google.com", // Test author
		},
		BugComponent: "b:753367",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}},
		Data: []string{"autoplay_allowed.html", "audio.mp3"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AutoplayAllowed{}, pci.VerifiedFunctionalityJS),
		},
	})
}

// AutoplayAllowed tests the AutoplayAllowed policy.
func AutoplayAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	for _, param := range []struct {
		name        string
		wantPlaying bool
		policy      *policy.AutoplayAllowed
	}{
		{
			name:        "deny",
			wantPlaying: false,
			policy:      &policy.AutoplayAllowed{Val: false},
		},
		{
			name:        "allow",
			wantPlaying: true,
			policy:      &policy.AutoplayAllowed{Val: true},
		},
		{
			name:        "unset",
			wantPlaying: false,
			policy:      &policy.AutoplayAllowed{Stat: policy.StatusUnset},
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

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open the website with the media.
			conn, err := br.NewConn(ctx, server.URL+"/autoplay_allowed.html")
			if err != nil {
				s.Fatal("Failed to open website: ", err)
			}
			defer conn.Close()

			var playing bool
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				// Check if the media is playing.
				if err := conn.Eval(ctx, "isMediaPlaying()", &playing); err != nil {
					return testing.PollBreak(err)
				}
				if !playing {
					return errNotPlaying
				}
				return nil
			}, &testing.PollOptions{Interval: 100 * time.Millisecond, Timeout: time.Second}); err != nil {
				if !errors.Is(err, errNotPlaying) {
					s.Fatal("Failed to request playing state: ", err)
				}
			}

			if playing != param.wantPlaying {
				s.Errorf("Unexpected media playing state: got %t; want %t", playing, param.wantPlaying)
			}
		})
	}
}
