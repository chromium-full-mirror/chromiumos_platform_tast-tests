// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MouseKeys,
		Desc: "Tests that Mouse keys moves mouse with keyboard",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"lkupo@google.com",             // Test author
		},
		BugComponent: "b:1688530",
		Timeout:      2 * time.Minute,
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInWithMouseKeys",
	})
}

func MouseKeys(ctx context.Context, s *testing.State) {

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

	// Enable Mouse Keys.
	cleanupMouseKeys, err := a11y.EnsureMouseKeysEnabled(ctx, tconn, true)
	if err != nil {
		s.Fatal("Failed to enable Mouse Keys setting: ", err)
	}
	defer func() {
		if err := cleanupMouseKeys(cleanupCtx); err != nil {
			s.Error("Failed to restore Mouse Keys setting during clean up: ", err)
		}
	}()

	// Slight delay to shorten the cursor movement granularity.
	delay := a11y.MouseKeysDefaultDelay + 100*time.Millisecond
	currentMouseButton := a11y.LeftMouseButton

	kb, err := input.KeyboardWithCustomDelay(ctx, delay)
	if err != nil {
		s.Fatalf("Failed to create a keyboard with delay %s: %v", delay, err)
	}

	defer kb.Close(cleanupCtx)

	showContextMenu := func(ctx context.Context) error {
		if currentMouseButton, err = a11y.ChangeMouseButton(ctx, kb, currentMouseButton, a11y.RightMouseButton); err != nil {
			return err
		}
		return kb.Type(ctx, a11y.MouseActionToKeyboardKey(a11y.MouseActionClick))
	}

	getMenuLocation := func(ctx context.Context) (*coords.Rect, error) {
		menuNode := nodewith.Role(role.Menu).First()
		if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(menuNode)(ctx); err != nil {
			s.Fatal("Context menu did not appear: ", err)
		}
		return ui.Location(ctx, menuNode)
	}

	if err := showContextMenu(ctx); err != nil {
		s.Fatal("Failed to show context menu: ", err)
	}

	initialMenuLocation, err := getMenuLocation(ctx)
	if err != nil {
		s.Fatal("Failed to get initial context menu location: ", err)
	}

	// Move the mouse upwards.
	upKey := a11y.MouseActionToKeyboardKey(a11y.MouseActionMoveUp)
	if err := kb.Type(ctx, strings.Repeat(upKey, 12)); err != nil {
		s.Fatal("Failed to move mouse upwards: ", err)
	}

	// Left click to exit out of the context menu.
	if currentMouseButton, err = a11y.ChangeMouseButton(ctx, kb, currentMouseButton, a11y.LeftMouseButton); err != nil {
		s.Fatal("Failed to change mouse button: ", err)
	}

	if err := kb.Type(ctx, a11y.MouseActionToKeyboardKey(a11y.MouseActionClick)); err != nil {
		s.Fatal("Failed to perform left click: ", err)
	}

	if err := showContextMenu(ctx); err != nil {
		s.Fatal("Failed to perform second right-click: ", err)
	}

	finalMenuLocation, err := getMenuLocation(ctx)
	if err != nil {
		s.Fatal("Failed to get final context menu location: ", err)
	}

	if initialMenuLocation.Top < finalMenuLocation.Top {
		s.Error("Failed to move cursor and context menu")
	}
}
