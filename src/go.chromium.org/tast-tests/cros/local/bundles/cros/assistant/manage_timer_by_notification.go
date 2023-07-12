// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManageTimerByNotification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests Assistant to manage timer by notification",
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Contacts:     []string{"assistive-eng@google.com"},
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      chrome.GAIALoginTimeout + time.Minute,
		Fixture:      "assistantWithGaia",
	})
}

func ManageTimerByNotification(ctx context.Context, s *testing.State) {
	fixtData := s.FixtValue().(*assistant.FixtData)
	cr := fixtData.Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	testPauseTimer(ctx, s, tconn)
	testCancelTimer(ctx, s, tconn)
	testStopTimer(ctx, s, tconn)
	testDisableAssistantWillRemoveTheTimerNotification(ctx, s, cr, tconn)
}

func testPauseTimer(ctx context.Context, s *testing.State, tconn *chrome.TestConn) {
	s.Log("Set a timer for three seconds")
	if _, err := assistant.SendTextQuery(ctx, tconn, "Set a timer for 3 seconds"); err != nil {
		s.Fatal("Failed to set a timer: ", err)
	}
	s.Log("Verifying the timer notification is visible")
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.ClassName("AshNotificationView"))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to appear: ", err)
	}

	s.Log("Clicking the Pause button on the notification")
	assistantNotification := nodewith.ClassName("AshNotificationView")
	pauseButton := nodewith.Name("Pause").Role(role.Button).Ancestor(assistantNotification)
	if err := ui.LeftClick(pauseButton)(ctx); err != nil {
		s.Fatal("Failed to click the Pause button: ", err)
	}
	s.Log("Show my timers")
	queryStatus, err := assistant.SendTextQuery(ctx, tconn, "Show my timers")
	if err != nil {
		s.Fatal("Failed to show timers: ", err)
	}
	s.Log("Verifying the show timers response")
	text := queryStatus.QueryResponse.TEXT
	if !strings.Contains(text, "paused") {
		s.Fatal("TEXT response doesn't contain the expected response of the text query: ", text)
	}

	// Also check that no notifications are shown in the UI.
	if err := ui.WaitUntilGone(nodewith.ClassName("AshNotificationView"))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to disappear: ", err)
	}

	// Clean up the timer.
	s.Log("Cancel the timer")
	queryStatus, err = assistant.SendTextQuery(ctx, tconn, "Cancel the timer")
	if err != nil {
		s.Fatal("Failed to cancel a timer: ", err)
	}
	s.Log("Verifying the cancel a timer response")
	text = queryStatus.QueryResponse.TEXT
	if !strings.Contains(text, "canceled") {
		s.Fatal("TEXT response doesn't contain the expected response of the text query: ", text)
	}

	confirmThatNoTimerIsSet(ctx, s, tconn)
}

func testCancelTimer(ctx context.Context, s *testing.State, tconn *chrome.TestConn) {
	s.Log("Set a timer for three seconds")
	if _, err := assistant.SendTextQuery(ctx, tconn, "Set a timer for 3 seconds"); err != nil {
		s.Fatal("Failed to set a timer: ", err)
	}
	s.Log("Verifying the timer notification is visible")
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.ClassName("AshNotificationView"))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to appear: ", err)
	}

	s.Log("Clicking the Cancel button on the notification")
	assistantNotification := nodewith.ClassName("AshNotificationView")
	cancelButton := nodewith.Name("Cancel").Role(role.Button).Ancestor(assistantNotification)
	if err := ui.LeftClick(cancelButton)(ctx); err != nil {
		s.Fatal("Failed to click the Cancel button: ", err)
	}

	// Also check that no notifications are shown in the UI.
	if err := ui.WaitUntilGone(nodewith.ClassName("AshNotificationView"))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to disappear: ", err)
	}

	confirmThatNoTimerIsSet(ctx, s, tconn)
}

func testStopTimer(ctx context.Context, s *testing.State, tconn *chrome.TestConn) {
	s.Log("Set a timer for three seconds")
	if _, err := assistant.SendTextQuery(ctx, tconn, "Set a timer for 3 seconds"); err != nil {
		s.Fatal("Failed to set a timer: ", err)
	}
	s.Log("Verifying the timer notification is visible")
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.ClassName("AshNotificationView"))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to appear: ", err)
	}

	s.Log("Waiting 3 seconds and verifying the timer started")
	assistantNotification := nodewith.ClassName("AshNotificationView")
	// Wait 3 seconds and check that Stop button is shown in the UI.
	if err := ui.WaitUntilExists(nodewith.Name("Stop").Role(role.Button).Ancestor(assistantNotification))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to appear: ", err)
	}
	// Also check that Pause and Cancel buttons are not shown in the UI.
	if err := ui.WaitUntilGone(nodewith.Name("Pause").Role(role.Button).Ancestor(assistantNotification))(ctx); err != nil {
		s.Fatal("Failed waiting for Pause button to disappear: ", err)
	}
	if err := ui.WaitUntilGone(nodewith.Name("Cancel").Role(role.Button).Ancestor(assistantNotification))(ctx); err != nil {
		s.Fatal("Failed waiting for Cancel button to disappear: ", err)
	}

	s.Log("Clicking the Stop button on the notification")
	stopButton := nodewith.Name("Stop").Role(role.Button).Ancestor(assistantNotification)
	if err := ui.LeftClick(stopButton)(ctx); err != nil {
		s.Fatal("Failed to click the Stop button: ", err)
	}

	// Also check that no notifications are shown in the UI.
	if err := ui.WaitUntilGone(nodewith.ClassName("AshNotificationView"))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to disappear: ", err)
	}

	confirmThatNoTimerIsSet(ctx, s, tconn)
}

func testDisableAssistantWillRemoveTheTimerNotification(ctx context.Context, s *testing.State, cr *chrome.Chrome, tconn *chrome.TestConn) {
	s.Log("Set a timer for three seconds")
	if _, err := assistant.SendTextQuery(ctx, tconn, "Set a timer for 3 seconds"); err != nil {
		s.Fatal("Failed to set a timer: ", err)
	}
	s.Log("Verifying the timer notification is visible")
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.ClassName("AshNotificationView"))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to appear: ", err)
	}

	s.Log("Disabling Assistant")
	if err := assistant.Disable(ctx, tconn); err != nil {
		s.Fatal("Failed to disable Assistant: ", err)
	}

	// Also check that no notifications are shown in the UI.
	if err := ui.WaitUntilGone(nodewith.ClassName("AshNotificationView"))(ctx); err != nil {
		s.Fatal("Failed waiting for notification to disappear: ", err)
	}
}

func confirmThatNoTimerIsSet(ctx context.Context, s *testing.State, tconn *chrome.TestConn) {
	s.Log("Show my timers")
	queryStatus, err := assistant.SendTextQuery(ctx, tconn, "Show my timers")
	if err != nil {
		s.Fatal("Failed to show timers: ", err)
	}
	s.Log("Verifying the show timers response")
	text := queryStatus.QueryResponse.TEXT
	if !strings.Contains(text, "It looks like you don't have any timers set at the moment.") {
		s.Fatal("TEXT response doesn't contain the expected response of the text query: ", text)
	}
}
