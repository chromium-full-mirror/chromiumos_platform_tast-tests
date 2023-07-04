// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/restrictionlevel"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/screenshare"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListScreenshareWindow,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with screen sharing restrictions while sharing a window",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"ayaelattar@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:hw_agnostic"},
		Data: []string{"text_1.html", "text_2.html"},
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Block users from sharing confidential information (screenshare_window): COM_DATPROT_CUJ3_TASK1_WF1.
			Value: "screenplay-54242ac5-8f9e-449e-98f7-a40ee9fa6630",
		}, {
			Key: "feature_id",
			// Block users from sharing confidential information within company (screenshare_window): COM_DATPROT_CUJ4_TASK1_WF1.
			Value: "screenplay-ed94f6c9-1142-495a-9dde-df73440d9d62",
		}, {
			Key: "feature_id",
			// Warn users from sharing confidential information (screenshare_window): COM_DATPROT_CUJ3_TASK2_WF1.
			Value: "screenplay-54aea56a-6acf-4fbf-b217-c4c9cbc8a6b9",
		}, {
			Key: "feature_id",
			// Warn users from sharing confidential information within company (screenshare_window): COM_DATPROT_CUJ4_TASK2_WF1.
			Value: "screenplay-1e0b14ae-d9d3-4ae7-9962-0e6d342f58a1",
		}},
		Params: []testing.Param{{
			Name:      "ash_blocked",
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: screenshare.TestParams{
				Name:        "blocked",
				Restriction: restrictionlevel.Blocked,
				Path:        screenshare.RestrictedPath,
				BrowserType: browser.TypeAsh,
			},
		}, {
			Name: "ash_allowed",
			ExtraAttr: []string{
				"group:golden_tier",
				"group:medium_low_tier",
				"group:hardware",
				"group:complementary"},
			Fixture: fixture.ChromePolicyLoggedIn,
			Val: screenshare.TestParams{
				Name:        "allowed",
				Restriction: restrictionlevel.Allowed,
				Path:        screenshare.UnrestrictedPath,
				BrowserType: browser.TypeAsh,
			},
		}, {
			Name:      "ash_warn_proceeded",
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: screenshare.TestParams{
				Name:        "warn_proceeded",
				Restriction: restrictionlevel.WarnProceeded,
				Path:        screenshare.RestrictedPath,
				BrowserType: browser.TypeAsh,
			},
		}, {
			Name: "ash_warn_cancelled",
			ExtraAttr: []string{
				"group:golden_tier",
				"group:medium_low_tier",
				"group:hardware",
				"group:complementary"},
			Fixture: fixture.ChromePolicyLoggedIn,
			Val: screenshare.TestParams{
				Name:        "warn_cancelled",
				Restriction: restrictionlevel.WarnCancelled,
				Path:        screenshare.RestrictedPath,
				BrowserType: browser.TypeAsh,
			},
		}, {
			Name:              "lacros_blocked",
			ExtraAttr:         []string{"group:mainline", "informational", "group:criticalstaging"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: screenshare.TestParams{
				Name:        "blocked",
				Restriction: restrictionlevel.Blocked,
				Path:        screenshare.RestrictedPath,
				BrowserType: browser.TypeLacros,
			},
		}, {
			Name: "lacros_allowed",
			ExtraAttr: []string{
				"group:golden_tier",
				"group:medium_low_tier",
				"group:hardware",
				"group:complementary"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: screenshare.TestParams{
				Name:        "allowed",
				Restriction: restrictionlevel.Allowed,
				Path:        screenshare.UnrestrictedPath,
				BrowserType: browser.TypeLacros,
			},
		}, {
			Name:              "lacros_warn_proceeded",
			ExtraAttr:         []string{"group:mainline", "informational", "group:criticalstaging"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: screenshare.TestParams{
				Name:        "warn_proceeded",
				Restriction: restrictionlevel.WarnProceeded,
				Path:        screenshare.RestrictedPath,
				BrowserType: browser.TypeLacros,
			},
		}, {
			Name: "lacros_warn_cancelled",
			ExtraAttr: []string{
				"group:golden_tier",
				"group:medium_low_tier",
				"group:hardware",
				"group:complementary"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: screenshare.TestParams{
				Name:        "warn_cancelled",
				Path:        screenshare.RestrictedPath,
				Restriction: restrictionlevel.WarnCancelled,
				BrowserType: browser.TypeLacros,
			},
		},
		}})
}

func DataLeakPreventionRulesListScreenshareWindow(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	params := s.Param().(screenshare.TestParams)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	url := server.URL + params.Path
	unrestrictedURL := server.URL + screenshare.UnrestrictedPath

	// Update the policy blob.
	pb := policy.NewBlob()
	if params.Restriction == restrictionlevel.Allowed || params.Restriction == restrictionlevel.Blocked {
		pb.AddPolicies(screenshare.GetScreenshareBlockPolicy(server.URL))
	} else {
		pb.AddPolicies(screenshare.GetScreenshareWarnPolicy(server.URL))
	}

	// Update policy.
	if err := policyutil.ServeBlobAndRefresh(ctx, fakeDMS, cr, pb); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	defer ash.CloseAllWindows(cleanupCtx, tconn)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, params.BrowserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer func(ctx context.Context) {
		if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
			// The Lacros browser is not closed in other places in the test.
			s.Error("The Lacros browser probably crashed: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+params.Name)

	var conn *browser.Conn
	if conn, err = br.NewConn(ctx, unrestrictedURL); err != nil {
		s.Fatal("Failed to open page: ", err)
	}

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		s.Fatalf("Failed to wait for %q to achieve quiescence: %v", unrestrictedURL, err)
	}

	var screenRecorder *uiauto.ScreenRecorder
	screenRecorder, err = uiauto.NewWindowRecorder(ctx, tconn /*windowIndex=*/, 0)

	if err != nil {
		s.Fatal("Failed to create ScreenRecorder: ", err)
	}

	if screenRecorder == nil {
		s.Fatal("Screen recorder was not found")
	}

	if err := screenRecorder.Start(ctx, tconn); err != nil {
		s.Fatal("Failed to start screen recorder: ", err)
	}

	defer uiauto.ScreenRecorderStopSaveRelease(ctx, screenRecorder, filepath.Join(s.OutDir(), "dlpScreenShare.mp4"))

	wantAllowed := params.Restriction == restrictionlevel.Allowed || params.Restriction == restrictionlevel.WarnProceeded

	// Screenshare should be allowed.
	if err := screenshare.CheckFrameStatus(ctx, screenRecorder, true); err != nil {
		s.Fatal("Failed to check frame status: ", err)
	}

	if conn, err = br.NewConn(ctx, url); err != nil {
		s.Fatal("Failed to open page: ", err)
	}

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		s.Fatalf("Failed to wait for %q to achieve quiescence: %v", url, err)
	}

	// The "Screen share paused" notification should appear if and only if the site is blocked.
	if _, err := ash.WaitForNotification(ctx, tconn, 5*time.Second, ash.WaitIDContains(screenshare.ScreensharePausedIDContains), ash.WaitTitle(screenshare.ScreensharePausedTitle)); (err != nil) == (params.Restriction == restrictionlevel.Blocked) {
		if err != nil {
			s.Errorf("Failed to wait for notification with title %q: %v", screenshare.ScreensharePausedTitle, err)
		} else {
			s.Errorf("Notification with title %q appeared when it should not have", screenshare.ScreensharePausedTitle)
		}
	}

	if params.Restriction == restrictionlevel.WarnProceeded {
		// Hit Enter, which is equivalent to clicking on the "Share anyway" button.
		if err := keyboard.Accel(ctx, "Enter"); err != nil {
			s.Fatal("Failed to hit Enter: ", err)
		}

		if _, err := ash.WaitForNotification(ctx, tconn, 10*time.Second, ash.WaitIDContains(screenshare.ScreenshareResumedIDContains), ash.WaitTitle(screenshare.ScreenshareResumedTitle)); err != nil {
			s.Errorf("Failed to wait for notification with title %q: %v", screenshare.ScreenshareResumedTitle, err)
		}

	} else if params.Restriction == restrictionlevel.WarnCancelled {
		// Hit Esc, which is equivalent to clicking on the "Cancel" button.
		if err := keyboard.Accel(ctx, "Esc"); err != nil {
			s.Fatal("Failed to hit Esc: ", err)
		}
	}

	// Close notifications.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	// Frame status value should be as per wantAllowed.
	if err := screenshare.CheckFrameStatus(ctx, screenRecorder, wantAllowed); err != nil {
		s.Fatal("Polling the frame status timed out: ", err)
	}

	if conn, err = br.NewConn(ctx, unrestrictedURL); err != nil {
		s.Fatal("Failed to open page: ", err)
	}

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		s.Fatalf("Failed to wait for %q to achieve quiescence: %v", unrestrictedURL, err)
	}

	if _, err := ash.WaitForNotification(ctx, tconn, 5*time.Second, ash.WaitIDContains(screenshare.ScreenshareResumedIDContains), ash.WaitTitle(screenshare.ScreenshareResumedTitle)); (err != nil) == (params.Restriction == restrictionlevel.Blocked) {
		if err != nil {
			s.Errorf("Failed to wait for notification with title %q: %v", screenshare.ScreenshareResumedTitle, err)
		} else {
			s.Errorf("Notification with title %q appeared when it should not have", screenshare.ScreenshareResumedTitle)
		}
	}

	// Screenshare should be allowed unless user cancelled sharing after a warning.
	if err := screenshare.CheckFrameStatus(ctx, screenRecorder, params.Restriction != restrictionlevel.WarnCancelled); err != nil {
		s.Fatal("Failed to check frame status: ", err)
	}

	// Once the user clicks "Share anyway", returning to the site later should allow for sharing without another prompt.
	if params.Restriction == restrictionlevel.WarnProceeded {
		if conn, err = br.NewConn(ctx, url); err != nil {
			s.Fatal("Failed to open page: ", err)
		}

		if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
			s.Fatalf("Failed to wait for %q to achieve quiescence: %v", unrestrictedURL, err)
		}

		if err := screenshare.CheckFrameStatus(ctx, screenRecorder /*wantAllowed=*/, true); err != nil {
			s.Fatal("Failed to check frame status: ", err)
		}
	}
}
