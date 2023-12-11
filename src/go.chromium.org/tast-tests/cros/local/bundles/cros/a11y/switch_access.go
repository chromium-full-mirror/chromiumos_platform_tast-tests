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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SwitchAccess,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Toggles the Switch Access accessibility feature and ensures the setup guide is shown",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1272578", // ChromeOS Public Tracker > Experiences > Accessibility > Features > Switch Access
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func SwitchAccess(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to setup test API connection: ", err)
	}

	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SwitchAccess, true); err != nil {
		s.Fatal("Failed to enable Switch Access: ", err)
	}
	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	defer func() {
		// Turn Switch Access off at the end of this test.
		if err := a11y.ClearFeature(cleanupCtx, tconn, a11y.SwitchAccess); err != nil {
			s.Fatal("Failed to disable Switch Access: ", err)
		}

		dialogText := nodewith.NameContaining("Are you sure you want to turn off").Onscreen()
		yesButton := nodewith.Name("Yes").Role(role.Button).ClassName("MdTextButton").Onscreen()
		if err := uiauto.Combine("close Switch Access dialog",
			ui.WaitUntilExists(dialogText),
			ui.WaitUntilExists(yesButton),
			ui.LeftClick(yesButton),
			ui.WaitUntilGone(dialogText),
			ui.WaitUntilGone(yesButton),
		)(cleanupCtx); err != nil {
			s.Fatal("Failed to close the Switch Access confirmation dialog: ", err)
		}

		if err := cr.ResetState(cleanupCtx); err != nil {
			s.Fatal("Failed to reset Chrome: ", err)
		}
	}()

	// Ensure that the Switch Access setup guide appears on-screen.
	text := nodewith.NameContaining("Switch Access setup guide").Role(role.StaticText).Onscreen()
	if err := ui.WaitUntilExists(text)(ctx); err != nil {
		s.Fatal("Failed to wait for the Switch Access setup guide to show: ", err)
	}
}
