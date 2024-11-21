// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SetAndClearGuest,
		Desc: "Test setting guest wallpaper is cleared on next login",
		Contacts: []string{
			"cros-p13n-eng@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
			"group:release-health",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

func SetAndClearGuest(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.GuestLogin(), chrome.ExtraArgs("--force-tablet-mode=clamshell"))
	if err != nil {
		s.Fatal("Failed to create chrome instance: ", err)
	}
	defer func(ctx context.Context) {
		// If the second chrome.New call fails, cr can be nil.
		if cr != nil {
			cr.Close(ctx)
		}
	}(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test api connection: ", err)
	}
	defer func(ctx context.Context) {
		if cr != nil {
			faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")
		}
	}(cleanupCtx)

	filename := "first_session_recording.webm"
	screenRecorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	// Note that this defer func may be called with the screen recorder from the first guest session or the second guest
	// session depending on when a fatal error occurs.
	defer func(ctx context.Context) {
		uiauto.StopAndSaveOnError(ctx, screenRecorder, filepath.Join(s.OutDir(), filename), s.HasError)
	}(cleanupCtx)

	ui := uiauto.New(tconn)

	if err := uiauto.Combine("Enable light mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleLightMode(ui))(ctx); err != nil {
		s.Fatal("Failed to enable light mode: ", err)
	}

	if err := uiauto.Combine("Verify and set wallpaper",
		verifyDefaultWallpaper(ui),
		selectAndVerifyWallpaper(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to verify and set wallpaper: ", err)
	}

	if err := logOutWithKeyboardShortcut()(ctx); err != nil {
		s.Fatal("Failed to log out: ", err)
	}

	// Close Chrome to do some cleanup.
	// It will log some errors, as the session is closed already.
	cr.Close(ctx)

	// KeepState is necessary because otherwise wallpaper is cleared even if it would not be on a real device.
	if cr, err = chrome.New(ctx, chrome.KeepState(), chrome.GuestLogin()); err != nil {
		s.Fatal("Failed to restart Chrome: ", err)
	}

	if tconn, err = cr.TestAPIConn(ctx); err != nil {
		s.Fatal("Failed to re-establish test API connection: ", err)
	}

	filename = "second_session_recording.webm"
	screenRecorder = uiauto.CreateAndStartScreenRecorder(ctx, tconn)

	ui = uiauto.New(tconn)

	if err := uiauto.NamedAction("Verify default wallpaper after sign-in", verifyDefaultWallpaper(ui))(ctx); err != nil {
		s.Fatal("Failed to re-verify default wallpaper after logout and login: ", err)
	}
}

func verifyDefaultWallpaper(ui *uiauto.Context) uiauto.Action {
	return uiauto.Combine("open wallpaper picker and verify default wallpaper",
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.WaitForWallpaperWithName(ui, "Default Wallpaper"),
		wallpaper.CloseWallpaperPicker(),
	)
}

func selectAndVerifyWallpaper(ui *uiauto.Context) uiauto.Action {
	return uiauto.Combine("select and verify wallpaper",
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, constants.ElementCollection),
		wallpaper.SelectImage(ui, constants.LightElementImage),
		wallpaper.WaitForWallpaperWithName(ui, constants.LightElementImage),
		wallpaper.CloseWallpaperPicker(),
	)
}

func logOutWithKeyboardShortcut() uiauto.Action {
	return func(ctx context.Context) error {

		process, err := ashproc.Root()
		if err != nil {
			return errors.Wrap(err, "failed to get Chrome process")
		}

		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get keyboard")
		}
		defer kb.Close(ctx)
		if err := kb.Accel(ctx, "Ctrl+Shift+Q"); err != nil {
			return errors.Wrap(err, "failed to send first Ctrl+Shift+Q")
		}
		if err := kb.Accel(ctx, "Ctrl+Shift+Q"); err != nil {
			return errors.Wrap(err, "failed to send second Ctrl+Shift+Q")
		}
		// Wait for existing Chrome process to stop.
		if err := procutil.WaitForTerminated(ctx, process, 30*time.Second); err != nil {
			return errors.Wrap(err, "timeout waiting for Chrome to shutdown")
		}
		// Chrome is supposed to automatically restart.
		if _, err := ashproc.WaitForRoot(ctx, 30*time.Second); err != nil {
			return errors.Wrap(err, "failed waiting for Chrome to restart")
		}

		return nil
	}
}
