// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/restrictionlevel"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Notification strings.
const (
	captureNotAllowedTitle = "Can't capture confidential content"
	captureNotAllowedID    = "screen_capture_dlp_blocked"
	captureTakenTitle      = "Screenshot taken"
	captureTakenID         = "capture_mode_notification"
)

// A struct containing parameters for different screenshot tests.
type screenshotTestParams struct {
	name                  string
	path                  string
	restriction           restrictionlevel.RestrictionLevel
	wantNotificationTitle string
	wantNotificationID    string
	browserType           browser.Type
}

// Different paths used for testing.
const (
	unrestrictedPath = "/text_1.html"
	restrictedPath   = "/text_2.html"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListScreenshot,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with screenshot restrictions",
		Contacts: []string{
			"chromeos-dlp@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:hw_agnostic"},
		Data:         []string{"text_1.html", "text_2.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Block users from sharing confidential information (screenshot): COM_DATPROT_CUJ3_TASK1_WF1.
				Value: "screenplay-715aee69-a224-4849-b7f8-0cefb6864d2d",
			}, {
				Key: "feature_id",
				// Block users from sharing confidential information within company (screenshot): COM_DATPROT_CUJ4_TASK1_WF1.
				Value: "screenplay-29bf949b-8392-41d1-8fca-07d0e8116875",
			}, {
				Key: "feature_id",
				// Warn users from sharing confidential information (screenshot): COM_DATPROT_CUJ3_TASK2_WF1.
				Value: "screenplay-51593f5b-b147-4032-b3d2-98acfa692e8a",
			}, {
				Key: "feature_id",
				// Warn users from sharing confidential information within company (screenshot): COM_DATPROT_CUJ4_TASK2_WF1.
				Value: "screenplay-59a4a2b5-1fb6-4cb4-93eb-b632681822e3",
			}},
		Params: []testing.Param{{
			Name:      "ash_blocked",
			ExtraAttr: []string{"group:golden_tier"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: screenshotTestParams{
				name:                  "blocked",
				path:                  restrictedPath,
				restriction:           restrictionlevel.Blocked,
				wantNotificationTitle: captureNotAllowedTitle,
				wantNotificationID:    captureNotAllowedID,
				browserType:           browser.TypeAsh,
			},
		}, {
			Name:      "ash_warn_proceeded",
			ExtraAttr: []string{"group:golden_tier"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: screenshotTestParams{
				name:                  "warn_proceded",
				path:                  restrictedPath,
				restriction:           restrictionlevel.WarnProceeded,
				wantNotificationTitle: captureTakenTitle,
				wantNotificationID:    captureTakenID,
				browserType:           browser.TypeAsh,
			},
		}, {
			Name:      "ash_warn_cancelled",
			ExtraAttr: []string{"group:mainline"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: screenshotTestParams{
				name:        "warn_cancelled",
				path:        restrictedPath,
				restriction: restrictionlevel.WarnCancelled,
				browserType: browser.TypeAsh,
			},
		}, {
			Name:              "lacros_blocked",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: screenshotTestParams{
				name:                  "blocked",
				path:                  restrictedPath,
				restriction:           restrictionlevel.Blocked,
				wantNotificationTitle: captureNotAllowedTitle,
				wantNotificationID:    captureNotAllowedID,
				browserType:           browser.TypeLacros,
			},
		}, {
			Name:              "lacros_warn_proceeded",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: screenshotTestParams{
				name:                  "warn_proceeded",
				path:                  restrictedPath,
				restriction:           restrictionlevel.WarnProceeded,
				wantNotificationTitle: captureTakenTitle,
				wantNotificationID:    captureTakenID,
				browserType:           browser.TypeLacros,
			},
		}, {
			Name:              "lacros_warn_cancelled",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: screenshotTestParams{
				name:        "warn_cancelled",
				path:        restrictedPath,
				restriction: restrictionlevel.WarnCancelled,
				browserType: browser.TypeLacros,
			},
		}},
	})
}

// screenshotBlockPolicy returns a DLP policy that blocks screen sharing.
func screenshotBlockPolicy(serverURL string) []policy.Policy {
	return []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Disable taking screenshots of confidential content",
				Description: "User should not be able to take screenshots of confidential content",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						serverURL + restrictedPath,
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "SCREENSHOT",
						Level: "BLOCK",
					},
				},
			},
		},
	},
	}
}

// screenshotWarnPolicy returns a DLP policy that warns before taking a screenshot.
func screenshotWarnPolicy(serverURL string) []policy.Policy {
	return []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Warn before taking a screenshot confidential content",
				Description: "User should be warned before taking a screenshot of confidential content",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						serverURL + restrictedPath,
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "SCREENSHOT",
						Level: "WARN",
					},
				},
			},
		},
	},
	}
}

func DataLeakPreventionRulesListScreenshot(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Update the policy blob.
	pb := policy.NewBlob()
	if s.Param().(screenshotTestParams).restriction == restrictionlevel.Allowed || s.Param().(screenshotTestParams).restriction == restrictionlevel.Blocked {
		pb.AddPolicies(screenshotBlockPolicy(server.URL))
	} else {
		pb.AddPolicies(screenshotWarnPolicy(server.URL))
	}

	// Update policy.
	if err := policyutil.ServeBlobAndRefresh(ctx, fakeDMS, cr, pb); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

	// Close previous notifications.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}
	// Clean up previous screenshots.
	if err := screenshot.RemoveScreenshots(downloadsPath); err != nil {
		s.Fatal("Failed to remove screenshots: ", err)
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(screenshotTestParams).browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer func(ctx context.Context) {
		if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
			// The Lacros browser is not closed in other places in the test.
			s.Error("The Lacros browser probably crashed: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+s.Param().(screenshotTestParams).name)

	url := server.URL + s.Param().(screenshotTestParams).path
	conn, err := br.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to open page: ", err)
	}
	defer conn.Close()

	testScreenshot(ctx, s, tconn, keyboard, downloadsPath, s.Param().(screenshotTestParams))
}

// testScreenshot attempts to take a screenshot, and reports errors if the behavior is different than expected.
func testScreenshot(ctx context.Context, s *testing.State, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, downloadsPath string, params screenshotTestParams) {
	layout, err := input.KeyboardTopRowLayout(ctx, keyboard)
	if err != nil {
		s.Fatal("Failed to retrieve keyboard top row layout: ", err)
	}

	// Press Ctrl+F5 to take the screenshot.
	if err := keyboard.Accel(ctx, "Ctrl+"+layout.SelectTask); err != nil {
		s.Fatal("Failed to type screenshot hotkey: ", err)
	}

	if params.restriction == restrictionlevel.WarnProceeded {
		// Hit Enter, which is equivalent to clicking on the "Capture anyway" button.
		if err := keyboard.Accel(ctx, "Enter"); err != nil {
			s.Fatal("Failed to hit Enter: ", err)
		}
	}

	if params.restriction == restrictionlevel.WarnCancelled {
		// Hit Esc, which is equivalent to clicking on the "Cancel" button.
		if err := keyboard.Accel(ctx, "Esc"); err != nil {
			s.Fatal("Failed to hit Esc: ", err)
		}

		// Ensure that a screenshot was not taken.
		if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitIDContains(captureTakenID), ash.WaitTitle(captureTakenTitle)); err == nil {
			s.Error("\"" + captureTakenTitle + "\" notification appeared after user cancelled action")
		}
	} else {
		// Restriction levels other than WarnCancelled should all result in a notification appearing, either for the screenshot being taken or blocked.
		if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitIDContains(params.wantNotificationID), ash.WaitTitle(params.wantNotificationTitle)); err != nil {
			s.Errorf("Failed to wait for notification with title \"%q\": %v", params.wantNotificationTitle, err)
		}
	}

	// Check if a screenshot was taken.
	takenScreenshot, err := screenshot.HasScreenshots(downloadsPath)
	if err != nil {
		s.Fatal("Failed to check if a screenshot was taken: ", err)
	}

	wantScreenshotTaken := (params.restriction == restrictionlevel.Allowed) || (params.restriction == restrictionlevel.WarnProceeded)

	if takenScreenshot != wantScreenshotTaken {
		s.Errorf("Unexpected screenshot allowed: got %v; want %v", takenScreenshot, wantScreenshotTaken)
	}
}
