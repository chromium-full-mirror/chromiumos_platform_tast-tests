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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoCaptureAllowedUrls,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of VideoCaptureAllowedUrls policy, checking that allow URLs don't request for video capture access",
		Contacts: []string{
			"chromeos-camera-app-eng@google.com",
			"wtlee@google.com",
			"eariassoto@google.com", // Test author
		},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		Data:         []string{"video_capture_allowed.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.VideoCaptureAllowedUrls{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// VideoCaptureAllowedUrls tests the VideoCaptureAllowedUrls policy.
func VideoCaptureAllowedUrls(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Error("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	for _, param := range []struct {
		name        string
		expectedAsk bool // expectedAsk states whether a dialog to ask for permission should appear or not.
		policy      *policy.VideoCaptureAllowedUrls
	}{
		{
			name:        "include_url",
			expectedAsk: false,
			policy:      &policy.VideoCaptureAllowedUrls{Val: []string{server.URL + "/video_capture_allowed.html"}},
		},
		{
			name:        "exclude_url",
			expectedAsk: true,
			policy:      &policy.VideoCaptureAllowedUrls{Val: []string{"https://my_corp_site.com/conference.html"}},
		},
		{
			name:        "unset",
			expectedAsk: true,
			policy:      &policy.VideoCaptureAllowedUrls{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Error("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Error("Failed to update policies: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open the test website.
			conn, err := cr.NewConn(ctx, server.URL+"/video_capture_allowed.html")
			if err != nil {
				s.Error("Failed to open website: ", err)
			}
			defer conn.Close()

			allowButton := nodewith.Name("Allow").Role(role.Button)
			if param.expectedAsk {
				if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(allowButton)(ctx); err != nil {
					s.Error("Failed to find the video capture prompt dialog: ", err)
				}
			} else {
				if err := ui.EnsureGoneFor(allowButton, 10*time.Second)(ctx); err != nil {
					s.Error("Failed to make sure no video capture prompt dialog shows: ", err)
				}
			}
		})
	}
}
