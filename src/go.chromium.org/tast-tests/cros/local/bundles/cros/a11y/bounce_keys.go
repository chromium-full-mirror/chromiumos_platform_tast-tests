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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: BounceKeys,
		Desc: "Tests that Bounce Keys properly debounces rapid key presses",
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

func BounceKeys(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	ui := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	// Enable Bounce Keys.
	cleanupBounceKeys, err := a11y.EnsureBounceKeysEnabled(ctx, tconn, true)
	if err != nil {
		s.Fatal("Failed to enable Bounce Keys setting: ", err)
	}
	defer func() {
		if err := cleanupBounceKeys(cleanupCtx); err != nil {
			s.Error("Failed to restore Bounce Keys setting during clean up: ", err)
		}
	}()

	ta, err := a11y.OpenTextAreaTab(ctx, cr, ui)
	if err != nil {
		s.Fatal("Failed to open textarea tab: ", err)
	}
	defer ta.Close(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	typeWithInterval := func(ctx context.Context, text string, interval time.Duration) error {
		for _, r := range text {
			c := string(r)
			if err := kb.Type(ctx, c); err != nil {
				return errors.Wrapf(err, "failed to type %q", c)
			}
			// GoBigSleepLint: Sleep to type with specified interval.
			if err := testing.Sleep(ctx, interval); err != nil {
				return errors.Wrapf(err, "failed to sleep for %v", interval)
			}
		}
		return nil
	}

	subtests := []struct {
		name     string
		input    string
		expected string
		interval time.Duration
	}{
		{
			name:     "Type same key slowly",
			input:    "aaaaa",
			expected: "aaaaa",
			interval: a11y.BounceKeysDefaultDelay + 100*time.Millisecond,
		},
		{
			name:     "Type same key quickly",
			input:    "bbbbb",
			expected: "b",
			interval: a11y.BounceKeysDefaultDelay - 200*time.Millisecond,
		},
		{
			name:     "Type alternating keys quickly",
			input:    "ghghgh",
			expected: "ghghgh",
			interval: a11y.BounceKeysDefaultDelay - 200*time.Millisecond,
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

			if err := typeWithInterval(ctx, subtest.input, subtest.interval); err != nil {
				s.Fatal("Failed to type with interval: ", err)
			}
			if err := ta.WaitForText(ctx, subtest.expected); err != nil {
				s.Fatalf("Failed waiting for expected text %q: %v", subtest.expected, err)
			}
		})
	}
}
