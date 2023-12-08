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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// htmlFile is the file containing the HTML+JS code exercising getDisplayMedia().
	tapCaptureAllowedByOriginsHTML = "tap_capture_allowed_by_origins.html"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabCaptureAllowedByOrigins,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of TabCaptureAllowedByOrigins policy",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"dandrader@google.com", // Test author
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
		Data: []string{tapCaptureAllowedByOriginsHTML},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.TabCaptureAllowedByOrigins{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.ScreenCaptureAllowed{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// TabCaptureAllowedByOrigins tests the namesake policy.
func TabCaptureAllowedByOrigins(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	for _, tc := range []struct {
		name     string
		policies []policy.Policy // list of policies to be set.
	}{
		{
			name: "not_set_block",
			policies: []policy.Policy{
				&policy.ScreenCaptureAllowed{Val: false},
			},
		},
		{
			name: "set_allow",
			policies: []policy.Policy{
				&policy.ScreenCaptureAllowed{Val: false},
				&policy.TabCaptureAllowedByOrigins{Val: []string{server.URL}},
			},
		},
		{
			name:     "no_policy_allow_all",
			policies: []policy.Policy{},
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, tc.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the test page.
			conn, err := br.NewConn(ctx, server.URL+"/"+tapCaptureAllowedByOriginsHTML)
			if err != nil {
				s.Fatal("Failed to connect to the window capture page: ", err)
			}
			defer conn.Close()

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+tc.name)

			ui := uiauto.New(tconn)
			mediaPicker := nodewith.Role(role.Window).ClassName("DesktopMediaPickerDialogView")
			// When there are no restrictions imposed by the presence of the policy, the
			// media picker will have a tab strip to display "Chrome Tab", "Window" and
			// "Entire Screen" tabs.
			tabbedPane := nodewith.ClassName("TabbedPane").Ancestor(mediaPicker)

			switch tc.name {
			case "not_set_block":
				// No media picker should come up.
				if err := ui.EnsureGoneFor(mediaPicker, 10*time.Second)(ctx); err != nil {
					s.Fatal("A media picker dialog appeared even though screen capture was disallowed: ", err)
				}
			case "set_allow":
				if err := ui.WaitUntilExists(mediaPicker)(ctx); err != nil {
					s.Fatal("Failed to find a media picker: ", err)
				}
				// Since the policy is present and the page is in its allow list, only
				// chrome tabs can be selected. Therefore a tabbed pane is not needed
				// because neither "Window" nor "Entire Screen" options will be available.
				if err := ui.EnsureGoneFor(tabbedPane, 10*time.Second)(ctx); err != nil {
					s.Fatal("An unrestricted media picker (ie, with a tab strip) was displayed: ", err)
				}
			default: // "no_policy_allow_all"
				if err := ui.WaitUntilExists(mediaPicker)(ctx); err != nil {
					s.Fatal("Failed to find a media picker: ", err)
				}
				// Checking for just for the existence of this tabbed pane is enough.
				if err := ui.WaitUntilExists(tabbedPane)(ctx); err != nil {
					s.Fatal("Failed to find a tabbed pane in media picker: ", err)
				}
			}
		})
	}

}
