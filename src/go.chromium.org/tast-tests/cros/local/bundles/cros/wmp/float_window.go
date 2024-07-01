// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FloatWindow,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that the float shortcut works on a floatable window",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > FloatingWindow
		BugComponent: "b:1252568",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "arc", "gaia"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 2*time.Minute,
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-f4dbfe9b-7a0d-4759-885a-79b1925d6cd0",
		}},
	})
}

// FloatWindow floats an open app window using the keyboard shortcut.
func FloatWindow(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.EnableFeatures("WindowLayoutMenu"),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Ensure there is no window open before test starts.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close any existing windows: ", err)
	}

	browserApp, err := apps.ChromeOrChromium(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find Chrome or Chromium app: ", err)
	}

	appsList := []apps.App{browserApp, apps.FilesSWA}

	// Set up ARC.
	if arcType, ok := arc.Type(); ok && arcType == arc.VM {
		if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
			s.Fatal("Failed to optin to Play Store and Close: ", err)
		}

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer a.Close(cleanupCtx)

		if err := a.WaitIntentHelper(ctx); err != nil {
			s.Fatal("Failed to wait for ARC Intent Helper: ", err)
		}

		appsList = append(appsList, apps.PlayStore)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard: ", err)
	}
	defer kb.Close(ctx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	for _, app := range appsList {
		s.Logf("Running %v through the test", app.Name)

		if err := apps.Launch(ctx, tconn, app.ID); err != nil {
			s.Fatalf("Failed to launch %s: %v", app.Name, err)
		}
		if err := ash.WaitForApp(ctx, tconn, app.ID, time.Minute); err != nil {
			s.Fatalf("Failed to wait for app %s to appear in shelf after launch: %v", app.Name, err)
		}

		window, err := ash.WaitForAnyWindow(ctx, tconn, func(w *ash.Window) bool {
			return w.AppID == app.ID && w.IsVisible && !w.IsAnimating
		})
		if err != nil {
			s.Fatalf("Failed to wait for %s to be visible and stop animating: %v", app.Name, err)
		}

		if err := window.ActivateWindow(ctx, tconn); err != nil {
			s.Fatalf("Failed to activate the %s window: %v", app.Name, err)
		}

		// Set the app to normal state so we can check unfloating goes
		// back to normal state.
		if err := ash.SetWindowStateAndWait(ctx, tconn, window.ID, ash.WindowStateNormal); err != nil {
			s.Fatalf("Failed to set the %s window state to \"Normal\": %v", app.Name, err)
		}

		if err := kb.Accel(ctx, "Search+Alt+F"); err != nil {
			s.Fatal("Failed to input float accelerator: ", err)
		}

		if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
			return w.ID == window.ID && w.State == ash.WindowStateFloated && !w.IsAnimating
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			s.Fatalf("Failed to get the correct app window state: got %s, want %s", window.State, ash.WindowStateFloated)
		}

		if err := kb.Accel(ctx, "Search+Alt+F"); err != nil {
			s.Fatal("Failed to input unfloat accelerator: ", err)
		}

		if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
			return w.ID == window.ID && w.State == ash.WindowStateNormal && !w.IsAnimating
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			s.Fatalf("Failed to get the correct app window state: got %s, want %s", window.State, ash.WindowStateNormal)
		}
	}
}
