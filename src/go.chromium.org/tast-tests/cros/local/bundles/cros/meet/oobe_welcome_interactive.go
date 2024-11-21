// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: OOBEWelcomeInteractive,
		Desc: "Test that clicks through dialogs on the OOBE welcome screen and checks that they are present and interactive",
		Contacts: []string{
			"core-devices@google.com",
			"joshuapius@google.com", // Test author
		},
		BugComponent: "b:341064525", // Communications > Video (Meet) > Platforms > Rooms > Core Devices (OS & Hardware)
		Attr:         []string{"group:meet", "group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "meet_device"},
		Timeout:      chrome.LoginTimeout + 45*time.Second,
	})
}

func OOBEWelcomeInteractive(ctx context.Context, s *testing.State) {
	tags := []string{
		"login_display_host*=4",
		"oobe_ui=4",
	}

	opts := append([]chrome.Option{
		chrome.ExtraArgs("--enable-logging", "--vmodule="+strings.Join(tags, ","))},
		chrome.NoLogin())
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	conn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to wait for OOBE connection: ", err)
	}
	defer conn.Close()

	if err := conn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the Welcome screen to be visible: ", err)
	}

	// Get path to current dialog
	var welcomeRoot string
	const f = `activeScreen = OobeAPI.getCurrentScreenName();
                activeStep = OobeAPI.getCurrentScreenStep();
                "document.querySelector('#"+activeScreen+"').shadowRoot.querySelector('[for-step="+activeStep+"]').shadowRoot"`
	if err := conn.Eval(ctx, f, &welcomeRoot); err != nil {
		s.Fatal("Failed to get welcome dialog root: ", err)
	}

	// Get list of button options
	var buttonOptions []string
	exp := fmt.Sprintf(`Array.from(%s.querySelectorAll("oobe-icon-button"))
		.map((e) => e.id)`, welcomeRoot)

	if err := conn.Eval(ctx, exp, &buttonOptions); err != nil {
		s.Fatal("Failed to get dialog button list: ", err)
	}

	dialogMap := make(map[string]string)

	initialInfo := getActiveScreen(ctx, conn, s)
	dialogMap[initialInfo] = "N/A"

	// Iteratively attempt to open each screen and ensure that a new, unique dialog / page is opened
	for _, buttonID := range buttonOptions {
		openedScreen := checkButtonInteractive(ctx, conn, s, buttonID, initialInfo, welcomeRoot)

		if dupVal, dup := dialogMap[openedScreen]; dup {
			s.Fatalf("Failed to open unique screen from button of ID %s; found reused screen %s (same screen as button of ID %s)",
				buttonID, openedScreen, dupVal)
		}
		dialogMap[openedScreen] = buttonID
	}
}

// getActiveScreen returns a string with the active screen name, dialog, and step
func getActiveScreen(ctx context.Context, conn *chrome.Conn, s *testing.State) string {
	var activeDialog string
	var activeScreen string
	var activeStep string

	const getDialog = `dialog = OobeAPI.getOobeActiveDialog();
		(dialog != null) ? dialog.id : 'null'`
	if err := conn.Eval(ctx, getDialog, &activeDialog); err != nil {
		s.Fatal("Failed to get active OOBE dialog: ", err)
	}
	if err := conn.Eval(ctx, "OobeAPI.getCurrentScreenName()", &activeScreen); err != nil {
		s.Fatal("Failed to get active OOBE screen name: ", err)
	}
	if err := conn.Eval(ctx, "OobeAPI.getCurrentScreenStep()", &activeStep); err != nil {
		s.Fatal("Failed to get active OOBE screen step: ", err)
	}

	return fmt.Sprintf("%s | %s | %s", activeScreen, activeDialog, activeStep)
}

// checkButtonInteractive attempts to open a dialog corresponding to buttonID, returns information regarding the opened dialog, and closes it
func checkButtonInteractive(ctx context.Context, conn *chrome.Conn, s *testing.State, buttonID, initialScreen, welcomeRoot string) string {
	f := fmt.Sprintf(`%s.querySelector('#%s').click()`, welcomeRoot, buttonID)

	if err := conn.Eval(ctx, f, nil); err != nil {
		s.Fatalf("Failed to select button of ID %s: %s", buttonID, err.Error())
	}

	activeScreen := getActiveScreen(ctx, conn, s)

	const exp = `screenName = OobeAPI.getCurrentScreenName()
		document.querySelector('#'+screenName).shadowRoot.querySelector('[slot=bottom-buttons]').querySelector('oobe-text-button').click()`
	if err := conn.Eval(ctx, exp, nil); err != nil {
		s.Fatalf("Failed to return to main page after navigating to button of ID %s: %s", buttonID, err.Error())
	}
	return activeScreen
}
