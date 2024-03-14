// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManualUI,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Allow manual power test to be performed and the test results to be collected to evaluate power metrics",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			// How many minutes to capture metrics.
			"power.test_duration",
		},
		Timeout: 24*time.Hour + power.RecorderTimeout,
		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: "powerAsh",
			},
			{
				Name:    "ash_arc",
				Fixture: "powerAshARC",
			},
			{
				Name:    "ash_gaia",
				Fixture: "powerAshGAIA",
			},
			{
				Name:              "lacros",
				Fixture:           "powerLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:              "lacros_arc",
				Fixture:           "powerLacrosARC",
				ExtraSoftwareDeps: []string{"lacros"},
			},

			{
				Name:              "lacros_gaia",
				Fixture:           "powerLacrosGAIA",
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

func ManualUI(ctx context.Context, s *testing.State) {

	const (
		manualSetupDuration         = 3 * time.Minute
		defaultTestDuration         = 5 * time.Minute
		setupTestUsedTimeMsgFormat  = "Please start setup for power test and complete within %v. %s"
		manualTestUsedTimeMsgFormat = "Please start manual testing and complete within %v. %s"
		manualCompletionMsg         = `Press "Ctrl + Search (Launcher) + S" on the DUT,
		or open the 'Key Shortcuts' app to indicate completion.`
	)

	manualTestDuration := defaultTestDuration
	if varValue, ok := s.Var("power.test_duration"); ok {
		value, err := strconv.Atoi(varValue)
		if err != nil {
			s.Fatal("Failed to parse power.test_duration: ", err)
		}
		if value <= 0 || value > 1440 {
			s.Fatal("Please enter a test duration between 1-30 (minutes)")
		}
		manualTestDuration = time.Duration(value) * time.Minute
	}

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	startSetupMsg := fmt.Sprintf(setupTestUsedTimeMsgFormat, manualSetupDuration, manualCompletionMsg)
	s.Log(startSetupMsg)

	// Notify testers with a pop-up notification that they can start setup procedure.
	if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic,
		"Manual Power Test Setup",
		startSetupMsg); err != nil {
		s.Log("Failed to create test start setup notification: ", err)
	}

	testing.ContextLog(ctx, "Start setting up the power recorder")
	r := power.NewRecorder(ctx, time.Second, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Failed to cooldown before the manual setup: ", err)
	}

	// Give a few minutes for testers to do manual testing setup.
	// Testers can press "Ctrl + Launcher + S" to bring up the keyboard
	// shortcuts widget to end the manual setup at any time.
	ui := uiauto.New(tconn)
	ksReg := regexp.MustCompile("[sS]hortcuts")
	keyboardShortcutsWindow := nodewith.NameRegex(ksReg).Role(role.Window).HasClass("BrowserFrame")
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(keyboardShortcutsWindow)
	if err := ui.WithTimeout(manualSetupDuration).WaitUntilExists(keyboardShortcutsWindow)(ctx); err == nil {
		if err := ui.LeftClick(closeButton)(ctx); err != nil {
			s.Error("Failed to close the Keyboard shortcuts widget: ", err)
		}
		msg := "You indicated the test setup is done. Test will proceed to next step."
		s.Log(msg)
		if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic,
			"Manual Power Test Setup Finish", msg); err != nil {
			s.Log("Failed to create test setup completion notification: ", err)
		}
	} else {
		msg := fmt.Sprintf("%v setup time has been reached. Test will proceed to next step.", manualSetupDuration)
		s.Log(msg)
		if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic,
			"Manual Power Test Setup Finish", msg); err != nil {
			s.Log("Failed to create test setup completion notification: ", err)
		}
	}

	if err := r.Cooldown(ctx); err != nil {
		s.Log("Failed to cooldown after the manual setup: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	startTestMsg := fmt.Sprintf(manualTestUsedTimeMsgFormat, manualTestDuration, manualCompletionMsg)
	s.Log(startTestMsg)

	// Notify testers with a pop-up notification that they can start manual testing.
	if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic,
		"Manual Power Test Start",
		startTestMsg); err != nil {
		s.Log("Failed to create test start notification: ", err)
	}

	// Given few minutes for testers to do manual testing.
	if err := ui.WithTimeout(manualTestDuration).WaitUntilExists(keyboardShortcutsWindow)(ctx); err == nil {
		msg := "You indicated the manual test is done. Test will collect the result."
		s.Log(msg)
		if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic,
			"Manual Power Test Finish", msg); err != nil {
			s.Log("Failed to create test completion notification: ", err)
		}
	} else {
		msg := fmt.Sprintf("%v manual test time has been reached. Test will collect the result.", manualTestDuration)
		s.Log(msg)
		if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic,
			"Manual Power Test Finish", msg); err != nil {
			s.Log("Failed to create test completion notification: ", err)
		}
	}

	// Clean up and close all open windows after all manual testing.
	if err := cuj.CloseAllWindows(ctx, tconn); err != nil {
		s.Log("Failed to close all windows: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
