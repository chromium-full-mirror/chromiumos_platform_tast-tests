// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LockScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Quick Settings button to launch Audio settings disabled on lock screen",
		// ChromeOS > Software > System Services > Peripherals > Audio (1321112)
		BugComponent: "b:1321112",
		Contacts: []string{
			"cros-peripherals@google.com",
			"ashleydp@google.com",
			"zentaro@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-816eefa8-76ad-43ec-8300-c747f4b59987",
			},
		},
	})
}

// LockScreen verifies button in QuickSettings to launch Audio settings
// is disabled on lock screen.
func LockScreen(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	const (
		username = "testuser@gmail.com"
		password = "pass"

		lockTimeout = 30 * time.Second
	)

	cr, err := chrome.New(ctx, chrome.FakeLogin(chrome.Creds{User: username, Pass: password}), chrome.EnableFeatures("QsRevamp"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Unlock the screen to ensure subsequent tests aren't affected by the screen remaining locked.
	defer func(ctx context.Context, tconn *chrome.TestConn) {
		keyboard, err := input.VirtualKeyboard(ctx)
		if err != nil {
			s.Fatal("Failed to get virtual keyboard: ", err)
		}
		if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, lockTimeout); err != nil {
			s.Fatalf("Waiting for screen to be ready for password failed: %v (last status %+v)", err, st)
		}

		if err := lockscreen.EnterPassword(ctx, tconn, username, password+"\n", keyboard); err != nil {
			s.Fatal("Entering password failed: ", err)
		}

		if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.LoggedIn }, 30*time.Second); err != nil {
			s.Fatalf("Failed waiting to log in: %v, last state: %+v", err, st)
		}
	}(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	cleanup, err := quicksettings.Init(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to init quicksettings: ", err)
	}
	defer cleanup()

	if err := quicksettings.OpenAudioSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open Quick Settings audio detail view")
	}
	testing.ContextLog(ctx, "Quick settings opened to audio detail view")

	settingsGearIcon := nodewith.HasClass("IconButton").Name("Audio settings")
	// Click on settings gear to open OS Settings at audio settings subpage.
	ui := uiauto.New(tconn).WithTimeout(2 * time.Second)
	if err := ui.CheckRestriction(settingsGearIcon, restriction.None)(ctx); err == nil {
		s.Fatal("Launch audio settings button has restriction")
	}
	testing.ContextLog(ctx, "Verified button not disabled")

	if err := quicksettings.LockScreen(ctx, tconn); err != nil {
		s.Fatal("Failed to lock screen")
	}
	testing.ContextLog(ctx, "Screen locked")

	if err := quicksettings.OpenAudioSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open Quick Settings to audio detail view")
	}
	testing.ContextLog(ctx, "Quick settings opened to audio detail view")

	if err := ui.CheckRestriction(settingsGearIcon, restriction.Disabled)(ctx); err == nil {
		s.Fatal("Launch audio settings button state missing the 'disabled' restriction")
	}
	testing.ContextLog(ctx, "Verified button disabled on lock screen")
}
