// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"sync"
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

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScreenCaptureAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that the ScreenCaptureAllowed policy is correctly applied",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"jityao@google.com", // Test author
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome", "lacros"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.LacrosPolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ScreenCaptureAllowed{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func ScreenCaptureAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	for _, param := range []struct {
		name  string
		value *policy.ScreenCaptureAllowed
	}{
		{
			name:  "enabled",
			value: &policy.ScreenCaptureAllowed{Val: true},
		},
		{
			name:  "disabled",
			value: &policy.ScreenCaptureAllowed{Val: false},
		},
		{
			name:  "unset",
			value: &policy.ScreenCaptureAllowed{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Open lacros browser.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeLacros)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			conn, err := br.NewConn(ctx, "")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}

			// Any HTTPS URL works.
			if err := conn.Navigate(ctx, "https://www.google.com"); err != nil {
				s.Fatal("Failed to navigate to https://www.google.com: ", err)
			}
			defer conn.Close()

			// Restrict each permission check to 5 seconds.
			uiCtx, uiCancel := context.WithTimeout(ctx, 5*time.Second)
			defer uiCancel()

			var wg sync.WaitGroup

			// Permission should be granted if policy is unset or if policy is enabled.
			expected := param.value.Stat == policy.StatusUnset || param.value.Val

			if expected {
				wg.Add(1)

				// Handle media selection source prompt in separate goroutine.
				go func() {
					defer wg.Done()

					// Connect to Test API to use it with the UI library.
					tconn, err := cr.TestAPIConn(uiCtx)
					if err != nil {
						s.Fatal("Failed to create Test API connection: ", err)
					}

					mediaPicker := nodewith.Role(role.Window).ClassName("DesktopMediaPickerDialogView")
					screenTab := nodewith.Name("Entire Screen").ClassName("TabbedPaneTab").Ancestor(mediaPicker)
					shareTarget := nodewith.ClassName("DesktopMediaSourceView").First()
					shareButton := nodewith.Name("Share").Role(role.Button).Ancestor(mediaPicker)

					ui := uiauto.New(tconn)

					// Click on "Entire Screen" tab, then on the desktop media source view, and then
					// click on Share button.
					if err := uiauto.Combine("Select media source",
						ui.WaitUntilExists(mediaPicker),
						ui.LeftClick(screenTab),
						ui.LeftClick(shareTarget),
						ui.LeftClick(shareButton),
					)(uiCtx); err != nil {
						s.Fatal("Failed to select media source: ", err)
					}
				}()
			}

			// Check that getDisplayMedia() permissions are correctly allowed or denied.
			actual := false
			if err := conn.Eval(uiCtx, `navigator.mediaDevices.getDisplayMedia()
                                .then(() => true)
                                .catch((err) => {
                                        if (err instanceof DOMException && err.message == "Permission denied") {
                                                return false;
                                        }
                                        throw  err;
                                })
                        `, &actual); err != nil {
				s.Fatal("Could not request for display media: ", err)
			}

			wg.Wait()

			if actual != expected {
				s.Fatalf("Unexpected permission granted status, expected %v, got %v", expected, actual)
			}
		})
	}
}
