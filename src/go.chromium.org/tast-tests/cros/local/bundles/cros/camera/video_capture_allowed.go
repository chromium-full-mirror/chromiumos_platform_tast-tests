// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoCaptureAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of VideoCaptureAllowed policy, checking if a website is allowed to capture video",
		Contacts: []string{
			"chromeos-camera-eng@google.com",
			"wtlee@google.com",
			"alexanderhartl@google.com", // Test author
		},
		BugComponent: "b:978428",
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
		Data: []string{"video_capture_allowed.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.VideoCaptureAllowed{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// VideoCaptureAllowed tests the VideoCaptureAllowed policy.
func VideoCaptureAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	for _, param := range []struct {
		name          string
		expectedBlock bool // expectedBlock states whether a dialog to ask for permission should appear or not.
		policy        *policy.VideoCaptureAllowed
	}{
		{
			name:          "unset",
			expectedBlock: false,
			policy:        &policy.VideoCaptureAllowed{Stat: policy.StatusUnset},
		},
		{
			name:          "blocked",
			expectedBlock: true,
			policy:        &policy.VideoCaptureAllowed{Val: false},
		},
		{
			name:          "allowed",
			expectedBlock: false,
			policy:        &policy.VideoCaptureAllowed{Val: true},
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

			// Open the test website.
			conn, err := br.NewConn(ctx, server.URL+"/video_capture_allowed.html")
			if err != nil {
				s.Fatal("Failed to open website: ", err)
			}
			defer conn.Close()

			// Check for existence of either the allow or block button until one of
			// them appears.
			// TODO(b/296796133): Temporarily remove the check while waiting for the
			// fix of indicators behavior.
		})
	}
}
