// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FullscreenMagnifier,
		LacrosStatus: testing.LacrosVariantUnneeded, // Does not use the browser
		Desc:         "Toggles the fullscreen magnifier using the keyboard shortcut and ensures the feature is updated",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"katie@chromium.org",           // Test author
		},
		BugComponent: "b:1272762", // ChromeOS Public Tracker > Experiences > Accessibility > Features > Magnifiers
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func FullscreenMagnifier(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	defer a11y.ClearFeature(cleanupCtx, tconn, a11y.ScreenMagnifier)

	// Make sure the feature starts disabled.
	enabled, err := a11y.FeatureEnabled(ctx, tconn, a11y.ScreenMagnifier)
	if err != nil {
		s.Fatal("Failed to get screen magnifier enabled state: ", err)
	}
	if enabled {
		s.Fatal("Expected screen magnifier to be disabled but it was not")
	}

	// Open a connection to the keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Error with creating EventWriter from keyboard: ", err)
	}

	// Toggle the magnifier with Ctrl + Search + m.
	if err := kb.Accel(ctx, "Search+Ctrl+m"); err != nil {
		s.Fatal("Error when pressing the fullscreen magnifier keys: ", err)
	}

	// Make sure the dialog is shown to enable the feature.
	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	dialogText := nodewith.NameContaining("You pressed the keyboard shortcut for the full-screen magnifier").Onscreen()
	if err := ui.WaitUntilExists(dialogText)(ctx); err != nil {
		s.Fatal("Failed to wait for the Full-screen magnifier dialog to be shown: ", err)
	}

	// Accept the dialog using the button.
	continueButton := nodewith.Name("Continue").Role(role.Button).Onscreen()
	if err := uiauto.Combine("Accept magnifier dialog",
		ui.LeftClick(continueButton), ui.WaitUntilGone(dialogText))(ctx); err != nil {
		s.Fatal("Failed to accept the Full-screen magnifier dialog: ", err)
	}

	// Make sure the feature is now enabled.
	enabled, err = a11y.FeatureEnabled(ctx, tconn, a11y.ScreenMagnifier)
	if err != nil {
		s.Fatal("Failed to get screen magnifier enabled state: ", err)
	}
	if !enabled {
		s.Fatal("Expected screen magnifier to be enabled but it was not")
	}

	// Try to turn it off using the same keyboard shortcut.
	if err := kb.Accel(ctx, "Search+Ctrl+m"); err != nil {
		s.Fatal("Error when pressing the fullscreen magnifier keys: ", err)
	}

	// Make sure the feature is now disabled.
	enabled, err = a11y.FeatureEnabled(ctx, tconn, a11y.ScreenMagnifier)
	if err != nil {
		s.Fatal("Failed to get screen magnifier enabled state: ", err)
	}
	if enabled {
		s.Fatal("Expected screen magnifier to be disabled but it was not")
	}
}
