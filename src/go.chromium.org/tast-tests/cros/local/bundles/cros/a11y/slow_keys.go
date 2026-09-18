// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SlowKeys,
		Desc: "Tests that Slow Keys properly delays key presses",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"aluh@chromium.org",            // Test author
		},
		BugComponent: "b:1686419",
		Timeout:      5 * time.Minute,
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func SlowKeys(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	ui := uiauto.New(tconn)

	// Enable Slow Keys.
	if err := a11y.ToggleSlowKeysSetting(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enable Slow Keys setting: ", err)
	}
	defer func() {
		if err := a11y.ToggleSlowKeysSetting(ctx, tconn, false); err != nil {
			s.Error("Failed to disable Slow Keys setting during clean up: ", err)
		}
	}()

	// Open a browser tab with a text area for typing.
	textURL := a11y.URLFromHTML("<textarea rows=\"10\" cols=\"60\"></textarea>")
	conn, err := a11y.NewTabWithURL(ctx, cr, textURL)
	if err != nil {
		s.Fatal("Failed to open textarea URL: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	textFieldNode := nodewith.Role(role.TextField).Ancestor(nodewith.HasClass("ContentsWebView"))
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(textFieldNode)(ctx); err != nil {
		s.Fatal("Text field node did not appear: ", err)
	}
	if err := ui.WithTimeout(5 * time.Second).LeftClickUntilFocused(textFieldNode)(ctx); err != nil {
		s.Fatal("Failed to click and focus on text field: ", err)
	}

	// Helper functions for typing.
	deleteText := func(ctx context.Context, s *testing.State) {
		// Use a new keyboard with enough delay to actually type with slow keys.
		kb, err := input.KeyboardWithCustomDelay(ctx, a11y.SlowKeysDefaultDelay+50*time.Millisecond)
		if err != nil {
			s.Fatal("Failed to create a keyboard to delete text: ", err)
		}
		defer kb.Close(ctx)

		if err := kb.TypeSequence(ctx, []string{"Ctrl+a", "Backspace"}); err != nil {
			s.Error("Failed to type text deletion sequence: ", err)
		}
		textNode := nodewith.Role(role.StaticText).Ancestor(textFieldNode)
		if err := ui.WithTimeout(3 * time.Second).WaitUntilGone(textNode)(ctx); err != nil {
			s.Error("Failed to delete text: ", err)
		}
	}

	subtests := []struct {
		name     string
		input    string
		expected string
		delay    time.Duration
	}{
		{
			name:     "Long key presses",
			input:    "abcde",
			expected: "abcde",
			delay:    a11y.SlowKeysDefaultDelay + 50*time.Millisecond,
		},
		{
			name:     "Short key presses",
			input:    "hijkl",
			expected: "",
			delay:    a11y.SlowKeysDefaultDelay - 200*time.Millisecond,
		},
	}
	for _, subtest := range subtests {
		s.Run(ctx, subtest.name, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()
			defer deleteText(ctx, s)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, subtest.name)

			kb, err := input.KeyboardWithCustomDelay(ctx, subtest.delay)
			if err != nil {
				s.Fatalf("Failed to create a keyboard with delay %s: %v", subtest.delay, err)
			}
			defer kb.Close(cleanupCtx)

			if err := kb.Type(ctx, subtest.input); err != nil {
				s.Fatal("Failed to type string: ", err)
			}
			textNode := nodewith.Role(role.StaticText).Ancestor(textFieldNode)
			if subtest.expected != "" {
				textNode = textNode.Name(subtest.expected)
				if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(textNode)(ctx); err != nil {
					s.Fatalf("Text node with %q did not appear: %v", subtest.expected, err)
				}
			} else {
				if err := ui.WithTimeout(3 * time.Second).WaitUntilGone(textNode)(ctx); err != nil {
					s.Fatal("Expected text to be empty: ", err)
				}
			}
		})
	}
}
