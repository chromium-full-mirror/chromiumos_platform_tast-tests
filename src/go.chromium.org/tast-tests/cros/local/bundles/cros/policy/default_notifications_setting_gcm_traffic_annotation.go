// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
		Timeout: 3 * time.Minute,
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
		shouldFindAnnotation bool // whether to allow the permission request and whether traffic annotation should appear on log.
		value                *policy.DefaultNotificationsSetting
	}{
		{
			name:                 "unset_allow",
			expectAskPermission:  true,
			shouldFindAnnotation: true,
			value:                &policy.DefaultNotificationsSetting{Stat: policy.StatusUnset},
		},
		{
			name:                 "unset_deny",
			expectAskPermission:  true,
			shouldFindAnnotation: false,
			value:                &policy.DefaultNotificationsSetting{Stat: policy.StatusUnset},
		},
		{
			name:                 "allow",
			expectAskPermission:  false,
			shouldFindAnnotation: true,
			value:                &policy.DefaultNotificationsSetting{Val: 1}, // Allow sites to show desktop notifications.
		},
		{
			name:                 "deny",
			expectAskPermission:  false,
			shouldFindAnnotation: false,
			value:                &policy.DefaultNotificationsSetting{Val: 2}, // Do not allow any site to show desktop notifications.
		},
		{
			name:                 "ask_allow",
			expectAskPermission:  true,
			shouldFindAnnotation: true,
			value:                &policy.DefaultNotificationsSetting{Val: 3}, // Ask every time a site wants to show desktop notifications.
		},
		{
			name:                 "ask_deny",
			expectAskPermission:  true,
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

			recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
			defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording_"+param.name+".webm"), s.HasError)

			// Setup server to send notifications from.
			server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
			serverURL, err := url.Parse(server.URL)
			if err != nil {
				s.Fatal("Failed to parse test server URL: ", err)
			}
			serverURL.Path = filepath.Join(serverURL.Path, defaultNotificationsSettingGcmTrafficAnnotationHTML)
			url := serverURL.String()
			defer server.Close()

			// Open the net-export page and start logging with raw bytes.
			if err := annotations.StartLogging(ctx, cr, br, true); err != nil {
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
			subscribeButton := nodewith.Name("subscribe").Role(role.Button)
			allowButton := nodewith.Name("Allow").Role(role.Button)
			blockButton := nodewith.Name("Block").Role(role.Button)
			statusText := nodewith.Name("Status").Role(role.StaticText)
			generalErrorText := nodewith.NameContaining("Error").Role(role.StaticText)
			NotAllowedErrorText := nodewith.Name("NotAllowedError").Role(role.StaticText)

			// Click on the subscribe button to trigger permissions request and push notification subscription.
			if err := ui.DoDefault(subscribeButton)(ctx); err != nil {
				s.Fatal("Failed to click Subscribe button: ", err)
			}

			if param.expectAskPermission {
				if param.shouldFindAnnotation {
					if err := ui.DoDefault(allowButton)(ctx); err != nil {
						s.Fatal("Failed to click the allow button: ", err)
					}
					if strings.HasPrefix(param.name, "ask") {
						// When policy is set to "ask", the permission panel occasionally shows up twice. See crbug.com/614632.
						if err := ui.DoDefault(allowButton)(ctx); err != nil {
							s.Log("Failed to click additional allow button: ", err)
						}
					}
				} else {
					if err := ui.DoDefault(blockButton)(ctx); err != nil {
						s.Fatal("Failed to click the block button: ", err)
					}
				}
			} else {
				// The 5 seconds duration is an arbitrary picked timeout,
				// should be long enough to verify no prompt will appear.
				if err := ui.EnsureGoneFor(permissionWindow, 5*time.Second)(ctx); err != nil {
					s.Fatal("Failed to verify that prompts are not shown and permission is granted/denied automatically: ", err)
				}
			}

			if param.shouldFindAnnotation {
				// For lacros, the permission panel occasionally shows up again. Click "allow" if needed.
				if s.Param().(browser.Type) == browser.TypeLacros {
					if param.name == "ask_allow" {
						if err := ui.DoDefault(allowButton)(ctx); err != nil {
							s.Log("Failed to click additional allow button in lacros: ", err)
						}
					}
				}
				// Verify the status text has been replaced.
				if err := ui.WaitUntilGone(statusText)(ctx); err != nil {
					s.Fatal("Got unexpected Status text: ", err)
				}
				if err := ui.EnsureGoneFor(generalErrorText, 3*time.Second)(ctx); err != nil {
					s.Fatal("Got unexpected error text: ", err)
				}
			} else {
				// Check that registration fails when permission is blocked.
				// HTML should update with error message.
				if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(NotAllowedErrorText)(ctx); err != nil {
					s.Fatal("Failed to verify that registration fails due to permissions block: ", err)
				}
			}

			// Wait to allow GCM to log network calls. Network call is sometimes delayed in logging.
			var errorCheckingForToken error
			foundAnnotationErr := testing.Poll(ctx, func(ctx context.Context) error {
				isFound := false
				isFound, errorCheckingForToken = checkUIForMatchingGcmToken(ctx, cr, ui, annotationID)
				if errorCheckingForToken != nil {
					return testing.PollBreak(errorCheckingForToken)
				}
				if isFound {
					return nil
				}
				return errors.New("Annotation with matching token not found yet")
			}, &testing.PollOptions{
				Timeout:  10 * time.Second,
				Interval: 1 * time.Second,
			})

			// Check if there was an error when checking logs.
			// Based on policy value, annotations/tokens are not expected to be found in some cases.
			// Annotations will be checked based on the policy at the end of the test.
			if errorCheckingForToken != nil {
				s.Fatal("Failed to check for gcm token: ", foundAnnotationErr)
			}
			foundAnnotation := foundAnnotationErr == nil

			if err := annotations.StopLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to stop logging: ", err)
			}
			if param.shouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Got: %t. Expected: %t", foundAnnotation, param.shouldFindAnnotation)
			}
		})
	}
}

// checkUIForMatchingGcmToken checks if the webpage UI has text matching any tokens associated with gcm_registration annotation.
// Returns whether the annotation with the token matches the UI and if there was an error reading the log file.
func checkUIForMatchingGcmToken(ctx context.Context, cr *chrome.Chrome, ui *uiauto.Context, annotationID string) (bool, error) {
	gcmTokens, err := getGcmTokensFromLogs(ctx, cr, annotationID)
	if err != nil {
		return false, errors.Wrap(err, "failed to get raw bytes")
	}
	for _, gcmToken := range gcmTokens {
		// Check if webpage has text matching the GCM network request tokens.
		// If there is a match, return true.
		found, err := ui.IsNodeFound(ctx, nodewith.Name(gcmToken).Role(role.StaticText))
		if err != nil {
			return false, errors.Wrapf(err, "failed to check node for %s", gcmToken)
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// getGcmTokensFromLogs checks logs for gcm_registration annotation and returns the tokens associated with the subscription URL request.
func getGcmTokensFromLogs(ctx context.Context, cr *chrome.Chrome, annotation string) (tokens []string, err error) {
	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user's Download path")
	}
	downloadLocation := filepath.Join(downloadsPath, annotations.DownloadName)

	// Read the net export log file.
	logFile, err := os.ReadFile(downloadLocation)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open logfile")
	}

	// Identify the ids associated with the annotation id.
	annotationPrefix := fmt.Sprintf("\"traffic_annotation\":%s", annotation)
	reForID := regexp.MustCompile(fmt.Sprintf(`%s.*\"id\":(\d+)`, annotationPrefix))
	ids := reForID.FindAllSubmatch(logFile, -1)
	// Go through all ids and check if there is a byte sequence for a GCM subscription token.
	for _, id := range ids {
		// Capture the base64 string associated with the URL request. "dG9rZW49" decodes to "token=".
		reForBase64Token := regexp.MustCompile(fmt.Sprintf("bytes\":\"dG9rZW49(.+)\"}.*\"id\":%s", id[1]))
		tokenMatches := reForBase64Token.FindAllSubmatch(logFile, -1)
		// Decode base64 string and return token.
		for _, token := range tokenMatches {
			tokenDecoded, err := base64.StdEncoding.DecodeString(string(token[1]))
			if err == nil {
				tokens = append(tokens, string(tokenDecoded))
			} else {
				return nil, errors.Wrap(err, "failed to decode")
			}
		}
	}
	return tokens, nil
}
