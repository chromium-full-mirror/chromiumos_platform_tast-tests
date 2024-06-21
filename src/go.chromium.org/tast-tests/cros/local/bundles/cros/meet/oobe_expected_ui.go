// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OOBEExpectedUI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that text, icons, and buttons are displayed with the correct color, font-family, font-size, and background color",
		Contacts: []string{
			"core-devices@google.com",
			"egwuekwe@google.com", // Test author
		},
		BugComponent: "b:543707",
		Attr: []string{"group:meet", "group:mainline", "informational",
			"group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "meet_device"},
		Timeout:      chrome.LoginTimeout + 45*time.Second,
		Vars:         []string{},
	})
}

func OOBEExpectedUI(ctx context.Context, s *testing.State) {

	tags := []string{
		"login_display_host*=4",
		"oobe_ui=4",
	}

	opts := append([]chrome.Option{
		chrome.ExtraArgs("--enable-logging", "--vmodule="+strings.Join(tags, ","))},
		chrome.NoLogin())

	opts = append(opts, chrome.DontSkipOOBEAfterLogin(),
		chrome.RemoveNotification(false),
		chrome.DontWaitForCryptohome())

	cr, err := chrome.New(
		ctx,
		chrome.NoLogin())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	s.Log("Waiting for the welcome screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx,
		"OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}

	jsExp := `
	function queryShadowRoot(element, selector) {
		if (window.getComputedStyle(element).visibility === "hidden" ||
			element.offsetHeight == 0 ||
			element.offsetWidth == 0 ||
			element.hidden) {
			return []
		}

		let matches = element.shadowRoot ? Array.from(element.shadowRoot
                  .querySelectorAll(selector)) : [];

		if (element.shadowRoot) {
			let childShadowHosts = element.shadowRoot.querySelectorAll('*');
			childShadowHosts.forEach(child => {
				if (child.shadowRoot) {
					matches = matches.concat(queryShadowRoot(child,
                                          selector));
				}
			});
		}

		let childElements = element.querySelectorAll('*');
		childElements.forEach(child => {
			if (child.shadowRoot) {
				matches = matches.concat(queryShadowRoot(child, selector));
			}
		});

		return matches;
	}

	elements = queryShadowRoot(document.querySelector("#connect")
          .shadowRoot.querySelector("#welcomeScreen"), '#button');

	const expTextIcon = {"color": "rgb(26, 115, 232)",
		"font-family": '"Google Sans", Roboto, sans-serif',
		"font-size": "22px",
		"background-color": "rgba(0, 0, 0, 0)"};
	const expButton = {"color": "rgb(255, 255, 255)",
		"font-family": '"Google Sans", Roboto, sans-serif',
		"font-size": "22px",
		"background-color": "rgb(26, 115, 232)"};

	const errLog = [];

	elements.forEach(element => {
		if (element.className != 'action-button') {
			for (const aspect in expTextIcon) {
				var computed = window.getComputedStyle(element, null)
                                  .getPropertyValue(aspect);
				if (computed != expTextIcon[aspect]) {
					console.log("Element name:", element);
					console.log("Real:", computed);
					console.log("Aspect:", aspect);
					console.log("Expected:", expTextIcon[aspect]);
					errLog.push("'" + element.tagName + ": " + element.innerText + "' had '"
                                          + aspect + "' value: " + computed +
                                          " but expected: " + expTextIcon[aspect]);
				}
			}
		} else {
			for (const aspect in expButton) {
				var computed = window.getComputedStyle(element, null)
                                  .getPropertyValue(aspect);
				if (computed != expButton[aspect]) {
					console.log("Element name:", element);
					console.log("Real:", computed);
					console.log("Aspect:", aspect);
					console.log("Expected:", expButton[aspect]);
					errLog.push("'" + element.tagName + ": " + element.innerText + "' had '"
                                          + aspect + "' value: " + computed +
                                          " but expected: " + expButton[aspect]);
				}
			}
		}
	});

	if (errLog.length > 0) {
		throw new Error(errLog);
	}
	`

	if err := oobeConn.Eval(ctx, jsExp, nil); err != nil {
		s.Fatal("Elements did not match their expected values: ", err)
	}
}
