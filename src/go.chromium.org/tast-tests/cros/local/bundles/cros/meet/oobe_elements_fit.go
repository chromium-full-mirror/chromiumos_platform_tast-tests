// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/meet/constants"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OobeElementsFit,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that checks whether unexpected scrollbars have been added to any OOBE elements",
		Contacts: []string{
			"core-devices@google.com",
			"torikauffman@google.com", // Test author
		},
		BugComponent: "b:341064525", // Communications > Video (Meet) > Platforms > Rooms > Core Devices (OS & Hardware)
		Attr:         []string{"group:meet", "group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "meet_device"},
		Timeout:      chrome.LoginTimeout + 45*time.Second,
	})
}

func OobeElementsFit(ctx context.Context, s *testing.State) {
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

	// Wait for test touch controller screen to wake up
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var width int
		if err := conn.Eval(ctx, "screen.width", &width); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get screen width"))
		}

		var height int
		if err := conn.Eval(ctx, "screen.height", &height); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get screen height"))
		}

		var ratio float32
		if err := conn.Eval(ctx, "window.devicePixelRatio", &ratio); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get screen pixel ratio"))
		}

		if (width == constants.TouchControllerWidth && height == constants.TouchControllerHeight) || ratio != 1 {
			return nil
		}

		return errors.Errorf("Unexpected screen dimensions, width=%d, height=%d, ratio=%f", width, height, ratio)
	}, &testing.PollOptions{
		Timeout:  20 * time.Second,
		Interval: 2 * time.Second,
	}); err != nil {
		// Touch controller dimensions not found; resize page to test standard dimensions
		resize := fmt.Sprintf("document.body.style.height = '%dpx'; document.body.style.width = '%dpx';",
			constants.TouchControllerHeight, constants.TouchControllerWidth)
		if err := conn.Eval(ctx, resize, nil); err != nil {
			s.Fatal("Failed to resize screen: ", err)
		}
	}

	checkAllElementsScrollbar(ctx, conn, s)
}

// checkAllElementsScrollbar iterates over all elements on the page to check the scrollbar properties
func checkAllElementsScrollbar(ctx context.Context, conn *chrome.Conn, s *testing.State) {
	const f = `function checkElements(elems) {
		for (const e of elems) {
			if (window.getComputedStyle(e).visibility === "hidden") {
                                continue;
                        }
			e.scrollTo(10, 10);
			if (e.scrollTop > 0 || e.scrollLeft > 0) {
				throw new Error();
			} else if (e.children.length > 0) {
				checkElements(e.children)
			} else if (!!e.shadowRoot) {
				checkElements(e.shadowRoot.children)
			}
		}
	}
	checkElements(document.getElementsByTagName('*'));`

	if err := conn.Eval(ctx, f, nil); err != nil {
		s.Fatal("Unexpected scrollbar properties: want no scrolls; got scrollbar on element")
	}
}
