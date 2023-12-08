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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var errMediaNotPlaying = errors.New("media is not playing")

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutoplayAllowlist,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checking if autoplay is allowed on a website or not, depending on the value of the AutoplayAllowlist policy",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"fbeaufort@chromium.org", // Owner of the policy
			"chrome-media-ux@google.com",
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
		Data: []string{"autoplay_allowed.html", "audio.mp3"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AutoplayAllowed{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.AutoplayAllowlist{}, pci.VerifiedFunctionalityJS),
		},
	})
}

// AutoplayAllowlist tests the AutoplayAllowlist policy.
func AutoplayAllowlist(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	// The website we will use for testing that contains media.
	testWebsite := server.URL + "/autoplay_allowed.html"
	defer server.Close()

	for _, param := range []struct {
		name            string
		wantPlaying     bool
		policyAllowed   *policy.AutoplayAllowed
		policyAllowlist *policy.AutoplayAllowlist
	}{
		{
			name:            "unset",
			wantPlaying:     false,
			policyAllowed:   &policy.AutoplayAllowed{Stat: policy.StatusUnset},
			policyAllowlist: &policy.AutoplayAllowlist{Stat: policy.StatusUnset},
		},
		{
			name:            "unset and no effect",
			wantPlaying:     true,
			policyAllowed:   &policy.AutoplayAllowed{Val: true},
			policyAllowlist: &policy.AutoplayAllowlist{Stat: policy.StatusUnset},
		},
		{
			name:            "no effect",
			wantPlaying:     true,
			policyAllowed:   &policy.AutoplayAllowed{Val: true},
			policyAllowlist: &policy.AutoplayAllowlist{Val: []string{"https://www.example.com"}},
		},
		{
			name:            "url allowlisted",
			wantPlaying:     true,
			policyAllowed:   &policy.AutoplayAllowed{Val: false},
			policyAllowlist: &policy.AutoplayAllowlist{Val: []string{testWebsite}},
		},
		{
			name:            "url not allowlisted",
			wantPlaying:     false,
			policyAllowed:   &policy.AutoplayAllowed{Val: false},
			policyAllowlist: &policy.AutoplayAllowlist{Val: []string{"https://www.example.com"}},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policyAllowed, param.policyAllowlist}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name+".txt")

			// Open the website with the media.
			conn, err := br.NewConn(ctx, testWebsite)
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
					return errMediaNotPlaying
				}
				return nil
			}, &testing.PollOptions{Interval: 100 * time.Millisecond, Timeout: time.Second}); err != nil {
				if !errors.Is(err, errMediaNotPlaying) {
					s.Fatal("Failed to request playing state: ", err)
				}
			}

			if playing != param.wantPlaying {
				s.Errorf("Unexpected media playing state: got %t; want %t", playing, param.wantPlaying)
			}
		})
	}
}
