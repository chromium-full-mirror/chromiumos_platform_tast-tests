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
	cleanupSlowKeys, err := a11y.EnsureSlowKeysEnabled(ctx, tconn, true)
	if err != nil {
		s.Fatal("Failed to enable Slow Keys setting: ", err)
	}
	defer func() {
		if err := cleanupSlowKeys(cleanupCtx); err != nil {
			s.Error("Failed to restore Slow Keys setting during clean up: ", err)
		}
	}()

	ta, err := a11y.OpenTextAreaTab(ctx, cr, ui)
	if err != nil {
		s.Fatal("Failed to open textarea tab: ", err)
	}
	defer ta.Close(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

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
			defer func() {
				if err := ta.Clear(cleanupCtx); err != nil {
					s.Error("Failed to clear text area: ", err)
				}
			}()
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, subtest.name)

			if err := ta.Focus(ctx); err != nil {
				s.Fatal("Text field lost focus before typing: ", err)
			}

			kb.Delay = subtest.delay

			if err := kb.Type(ctx, subtest.input); err != nil {
				s.Fatal("Failed to type string: ", err)
			}
			if err := ta.WaitForText(ctx, subtest.expected); err != nil {
				s.Fatalf("Failed waiting for expected text %q: %v", subtest.expected, err)
			}
		})
	}
}
