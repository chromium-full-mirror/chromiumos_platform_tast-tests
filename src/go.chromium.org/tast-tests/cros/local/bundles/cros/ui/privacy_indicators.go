// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrivacyIndicators,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check if the privacy indicators view show up when entering Google Meet",
		Contacts:     []string{"cros-status-area-eng@google.com", "leandre@chromium.org"},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		VarDeps: []string{
			"ui.PrivacyIndicators.meet_code",
		},
		Timeout: 3 * time.Minute,
		Params: []testing.Param{{
			Fixture: "chromeLoggedInWithCalendarEvents",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacrosLoggedInWithCalendarEvents",
			Val:               browser.TypeLacros,
		}},
	})
}

func PrivacyIndicators(ctx context.Context, s *testing.State) {
	// Shorten context to allow for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}
	ui := uiauto.New(tconn)
	meetingCode := s.RequiredVar("ui.PrivacyIndicators.meet_code")

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Grant mic, camera and notification to Meet to suppress prompts during testing.
	if err = br.GrantPermissions(ctx, []string{"*://meet.google.com/*"},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
		browser.NotificationsContentSetting,
	); err != nil {
		s.Fatal("Failed to grant permissions for mic, camera and notification: ", err)
	}

	url := "https://meet.google.com/" + meetingCode
	meetConn, err := br.NewConn(ctx, url, browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to open the hangout meet website: ", err)
	}
	defer meetConn.Close()

	// Match window titles `Google Meet` and `meet.google.com`.
	meetRE := regexp.MustCompile(`\bMeet\b|\bmeet\.\b`)
	meetWindow, err := ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return meetRE.MatchString(w.Title) })
	if err != nil {
		s.Fatal("Failed to find the Meet window: ", err)
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to detect it is in tablet-mode or not: ", err)
	}
	s.Logf("Is in tablet-mode: %t", inTabletMode)

	var pc pointer.Context
	if inTabletMode {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to create a touch controller: ", err)
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)

	// Check if the account has been added to the Meet page.
	// There might be a timing issue that the account has been added to Lacros profile but not yet propagated to the web page
	// due to different ways of looking up the credentials. Browser uses OAuth, the web reads cookies instead.
	// If this happens, it fails to load the app page with the account. See crbug.com/1322246 for the details.
	// To get around it for recovery it gives a retry by reloading the page to the web page URL.
	account := nodewith.Role(role.StaticText).NameContaining(`@gmail.com`)
	if err = ui.WithTimeout(time.Second).WaitUntilExists(account)(ctx); err != nil {
		s.Log("Reload page to sign in")
		if err := br.ReloadActiveTab(ctx); err != nil {
			s.Fatal("Failed to reload page: ", err)
		}
		if err := meetConn.Navigate(ctx, url); err != nil {
			s.Fatal("Failed to navigate page: ", err)
		}
		if err = ui.WaitUntilExists(account)(ctx); err != nil {
			s.Fatal("Failed to sign in to the Meet window: ", err)
		}
	}

	if err := meetWindow.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to activate the Meet window: ", err)
	}

	privacyIndicators := nodewith.ClassName("PrivacyIndicatorsTrayItemView").First()
	if err := ui.WaitUntilExists(privacyIndicators)(ctx); err != nil {
		s.Fatal("Privacy indicators view does not show up as expected: ", err)
	}

	// Close the Meet window, expect privacy indicators view to be gone.
	if err := meetWindow.CloseWindow(cleanupCtx, tconn); err != nil {
		s.Error("Failed to close the meeting: ", err)
	}
	if err := ui.WaitUntilGone(privacyIndicators)(ctx); err != nil {
		s.Fatal("Privacy indicators view does not disappear as expected: ", err)
	}
}
