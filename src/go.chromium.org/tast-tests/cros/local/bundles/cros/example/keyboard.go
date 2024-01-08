// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Keyboard,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Demonstrates injecting keyboard events",
		Contacts:     []string{"tast-core@google.com", "hidehiko@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

func Keyboard(ctx context.Context, s *testing.State) {
	// Test Values
	const (
		html      = "<input id='text' type='text' label='example.Keyboard.TextBox' autofocus>"
		inputText = "Hello, world!"
	)

	// 1. Boilerplate setup + create tab with input form
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// The ui matching further down depends on clamshell mode.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to disable tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to set up the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	c, err := a11y.NewTabWithURL(ctx, br, a11y.URLFromHTML(html))
	if err != nil {
		s.Fatal("Failed to open a new tab with HTML: ", err)
	}
	defer c.Close()

	// 2. Wait for focus on text box, then enter input text value
	s.Log("Waiting for focus")
	textbox := nodewith.NameContaining("label='example.Keyboard.TextBox'").Role(role.StaticText).Onscreen()

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	if err := uiauto.Combine("Focus text box",
		ui.WaitUntilExists(textbox),
		// TODO(crbug.com/1291585): ui.FocusAndWait doesn't seem to work on Lacros. Timed out waiting for event.Focus to occur.
		// Since the input element has 'autofocus' attribute commenting the line below won't affect the test results.
		//ui.FocusAndWait(textbox),
	)(ctx); err != nil {
		s.Fatal("Failed to focus the text box: ", err)
	}

	s.Log("Finding and opening keyboard device")
	ew, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open keyboard device: ", err)
	}
	defer ew.Close(ctx)

	s.Logf("Injecting keyboard events for %q", inputText)
	if err = ew.Type(ctx, inputText); err != nil {
		s.Fatal("Failed to write events: ", err)
	}

	// 3. Assert inputted text matches expected value
	textboxWithContent := nodewith.State(state.Editable, true).Role(role.InlineTextBox).Name(inputText)
	if err := ui.WaitUntilExists(textboxWithContent)(ctx); err != nil {
		s.Fatal("Failed to verify text input: ", err)
	}

	const (
		pageText = "mittens"
		dataURL  = "data:text/plain," + pageText
		bodyExpr = "document.body.innerText"
	)
	s.Logf("Navigating to %q via omnibox", dataURL)
	if err := ew.Accel(ctx, "Ctrl+L"); err != nil {
		s.Fatal("Failed to write events: ", err)
	}
	if err := ew.Type(ctx, dataURL+"\n"); err != nil {
		s.Fatal("Failed to write events: ", err)
	}
	mittensOutput := nodewith.Name(pageText).Role(role.InlineTextBox)
	if err := ui.WaitUntilExists(mittensOutput)(ctx); err != nil {
		s.Fatal("Failed to verify page text: ", err)
	}

	// Not all Chromebooks have the same layout for the function keys.
	layout, err := input.KeyboardTopRowLayout(ctx, ew)
	if err != nil {
		s.Fatal("Failed to get keyboard mapping: ", err)
	}

	key := layout.ZoomToggle
	// If the key is empty it means it is not mapped
	if key != "" {
		if err := ew.Accel(ctx, key); err != nil {
			s.Fatal("Failed to write events: ", err)
		}
	}
}
