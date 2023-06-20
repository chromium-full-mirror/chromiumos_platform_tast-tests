// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
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

const defaultNotificationsSettingGcmTrafficAnnotationHTML = "default_notifications_setting_gcm_traffic_annotation.html"
const defaultNotificationsSettingGcmTrafficAnnotationServiceWorkerJs = "default_notifications_setting_gcm_traffic_annotation_service-worker.js"

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefaultNotificationsSettingGcmTrafficAnnotation,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of DefaultNotificationsSetting policy, checks for `gcm_registration` traffic annotation on allowing notifications on pop-up at different policy values",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"alexwchen@google.com", // Test author
		},
		BugComponent: "b:1129862",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
		Data:         []string{defaultNotificationsSettingGcmTrafficAnnotationHTML, defaultNotificationsSettingGcmTrafficAnnotationServiceWorkerJs},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		Timeout: 5 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DefaultNotificationsSetting{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// DefaultNotificationsSettingGcmTrafficAnnotation tests the
// DefaultNotificationsSetting policy and checks for `gcm_registration`
// traffic annotation when allowing notifications.
func DefaultNotificationsSettingGcmTrafficAnnotation(ctx context.Context, s *testing.State) {
	const annotationID = "61656965" // gcm_registration

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	for _, param := range []struct {
		name                 string
		expectAskPermission  bool // expectAskPermission states whether a dialog to ask for permission should appear or not.
		selectAllow          bool // whether to allow the permission request
		shouldFindAnnotation bool // whether traffic annotation should appear on log
		value                *policy.DefaultNotificationsSetting
	}{
		{
			name:                 "unset_allow",
			expectAskPermission:  true,
			selectAllow:          true,
			shouldFindAnnotation: true,
			value:                &policy.DefaultNotificationsSetting{Stat: policy.StatusUnset},
		},
		{
			name:                 "unset_deny",
			expectAskPermission:  true,
			selectAllow:          false,
			shouldFindAnnotation: false,
			value:                &policy.DefaultNotificationsSetting{Stat: policy.StatusUnset},
		},
		{
			name:                 "allow",
			expectAskPermission:  false,
			selectAllow:          false, // Not used by test.
			shouldFindAnnotation: true,
			value:                &policy.DefaultNotificationsSetting{Val: 1}, // Allow sites to show desktop notifications.
		},
		{
			name:                 "deny",
			expectAskPermission:  false,
			selectAllow:          false, // Not used by test.
			shouldFindAnnotation: false,
			value:                &policy.DefaultNotificationsSetting{Val: 2}, // Do not allow any site to show desktop notifications.
		},
		{
			name:                 "ask_allow",
			expectAskPermission:  true,
			selectAllow:          true,
			shouldFindAnnotation: true,
			value:                &policy.DefaultNotificationsSetting{Val: 3}, // Ask every time a site wants to show desktop notifications.
		},
		{
			name:                 "ask_deny",
			expectAskPermission:  true,
			selectAllow:          false,
			shouldFindAnnotation: false,
			value:                &policy.DefaultNotificationsSetting{Val: 3}, // Ask every time a site wants to show desktop notifications.
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Setup server to send notifications from.
			server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
			serverURL, err := url.Parse(server.URL)
			if err != nil {
				s.Fatal("Failed to parse test server URL: ", err)
			}
			serverURL.Path = filepath.Join(serverURL.Path, defaultNotificationsSettingGcmTrafficAnnotationHTML)
			url := serverURL.String()
			defer server.Close()

			// GoBigSleepLint: wait for Chrome startup GCM registrations to complete.
			// GCM registrations happen in the background and may be recognized as false positives by this test.
			// Wait 20 seconds to allow background registrations to occur before logging.
			if err := testing.Sleep(ctx, 20*time.Second); err != nil {
				s.Fatal("Failed while waiting for Chrome startup GCM registrations to complete: ", err)
			}

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			conn, err := br.NewConn(ctx, url)
			if err != nil {
				s.Fatal("Failed to open website: ", err)
			}
			defer conn.Close()
			defer conn.CloseTarget(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			ui := uiauto.New(tconn)
			permissionWindow := nodewith.HasClass("PermissionPromptBubbleBaseView").Role(role.Window)
			allowButton := nodewith.Name("Allow").Role(role.Button)
			blockButton := nodewith.Name("Block").Role(role.Button)

			// Click on the subscribe button to trigger permissions request and push notification subscription.
			// Save triggerTime for annotation check.
			triggerTime := time.Now()
			if err := ui.DoDefault(nodewith.Name("subscribe").Role(role.Button))(ctx); err != nil {
				s.Fatal("Failed to click Subscribe button: ", err)
			}

			if param.expectAskPermission {
				if param.selectAllow {
					if err := ui.DoDefault(allowButton)(ctx); err != nil {
						s.Fatal("Failed to click the allow button: ", err)
					}
					if strings.HasPrefix(param.name, "ask") {
						// When policy is set to "ask", the permission panel shows up twice. See crbug.com/614632.
						if err := ui.DoDefault(allowButton)(ctx); err != nil {
							s.Fatal("Failed to click the allow button: ", err)
						}
					}
				} else {
					if err := ui.DoDefault(blockButton)(ctx); err != nil {
						s.Fatal("Failed to click the block button: ", err)
					}
					// Check that registration fails when permission is blocked.
					// HTML should update with error message.
					if err := ui.WaitUntilExists(nodewith.Name("NotAllowedError: Registration failed - permission denied").Role(role.StaticText))(ctx); err != nil {
						s.Fatal("Failed to verify that registration fails due to permissions block: ", err)
					}
				}
			} else {
				// The 10 seconds duration is an arbitrary picked timeout,
				// should be long enough to verify no prompt will appear.
				if err := ui.EnsureGoneFor(permissionWindow, 10*time.Second)(ctx); err != nil {
					s.Fatal("Failed to verify that prompts are not shown and permission is granted/denied automatically: ", err)
				}
			}

			// Stop logging and check the logs for gcm_registration NetworkTrafficAnnotationTag. Filter out annotations before trigger.
			foundAnnotation, err := annotations.StopLoggingCheckLogsFilterByTriggerTime(ctx, cr, br, annotationID, triggerTime)
			if err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}
			if param.shouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Got: %t. Expected: %t", foundAnnotation, param.shouldFindAnnotation)
			}
		})
	}
}
