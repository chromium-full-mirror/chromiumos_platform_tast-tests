// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/settings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const text = `Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Curabitur varius, nulla ut varius sollicitudin, erat erat fermentum metus,
id porttitor mi lorem at magna.
`

type testParams struct {
	EnableSlowKeys bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: SlowKeysPower,
		Desc: "Power tests for Slow Keys feature",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"aluh@chromium.org",            // Test author
		},
		BugComponent: "b:1686419",
		Timeout:      15*time.Minute + power.RecorderTimeout,
		Attr:         []string{"group:crosbolt", "crosbolt_weekly"},
		Params: []testing.Param{{
			Name: "enabled",
			Val:  testParams{EnableSlowKeys: true},
		}, {
			Name: "baseline",
			Val:  testParams{EnableSlowKeys: false},
		}},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func SlowKeysPower(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	powerOpts := &setup.PowerTestOptions{
		Backlight:          setup.SetBacklight,
		KeyboardBrightness: setup.SetKbBrightnessToZero,
	}
	cleanup, _, err := setup.PowerTestSetup(ctx, "Slow Keys Power Setup", tconn, powerOpts)
	if err != nil {
		s.Fatal("setup.PowerTestSetup: ", err)
	}
	defer cleanup(cleanupCtx)

	ui := uiauto.New(tconn)

	if s.Param().(testParams).EnableSlowKeys {
		// Enable Slow Keys.
		if err := a11y.ToggleSlowKeysSetting(ctx, tconn, true); err != nil {
			s.Fatal("Failed to enable Slow Keys setting: ", err)
		}
		defer func() {
			if err := a11y.ToggleSlowKeysSetting(cleanupCtx, tconn, false); err != nil {
				s.Error("Failed to disable Slow Keys setting during clean up: ", err)
			}
		}()
	}

	// Disable auto repeat keys so that repeat keys don't interfere with the slow keys power test.
	cleanupKeyRepeat, err := settings.EnsureKeyRepeatEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to toggle repeat keys off: ", err)
	}
	defer func() {
		if err := cleanupKeyRepeat(cleanupCtx, tconn); err != nil {
			s.Error("Failed to restore repeat keys setting during clean up: ", err)
		}
	}()

	// Open a browser tab with a text area for typing.
	textURL := a11y.URLFromHTML("<textarea rows=\"10\" cols=\"80\"></textarea>")
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

	// Finder for the actual text node in the text field.
	textNode := nodewith.Role(role.StaticText).Ancestor(textFieldNode)

	kb, err := input.KeyboardWithCustomDelay(ctx, a11y.SlowKeysDefaultDelay+50*time.Millisecond)
	if err != nil {
		s.Fatal("Failed to create a keyboard with custom delay: ", err)
	}
	defer kb.Close(cleanupCtx)

	r := power.NewRecorder(ctx, 5*time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	s.Log("Starting cooldown")
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	for i := 0; i < 3; i++ {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, fmt.Sprintf("iteration_%d", i))

		if err := kb.Type(ctx, text); err != nil {
			s.Fatal("Failed to type test string: ", err)
		}

		if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(textNode.First())(ctx); err != nil {
			s.Fatal("Text node did not appear: ", err)
		}
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
