// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/wmp/wmputils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FloatWindowMultitaskMenu,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that the float multitask menu works",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > FloatingWindow
		BugComponent: "b:1252568",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 2*time.Minute,
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-f4dbfe9b-7a0d-4759-885a-79b1925d6cd0",
		}},
	})
}

// FloatWindowMultitaskMenu tests floating a window via the Multitask Menu.
func FloatWindowMultitaskMenu(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := browser.TypeAsh
	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(),
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.EnableFeatures("WindowLayoutMenu"),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Open a new tab in order to show Chrome UI.
	conn, err := br.NewConn(ctx, chrome.NewTabURL)
	if err != nil {
		s.Fatal(err, "Failed to open new Chrome window")
	}
	defer conn.Close()

	bw, err := wmputils.EnsureOnlyBrowserWindowOpen(ctx, tconn, bt)
	if err != nil {
		s.Fatal("Failed to ensure 1 browser window open: ", err)
	}

	pc := pointer.NewMouse(tconn)
	defer pc.Close(cleanupCtx)

	ui := uiauto.New(tconn)
	pollOpts := testing.PollOptions{Timeout: 30 * time.Second}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Set the Chrome window to normal state before testing the caption button actions.
	if err := ash.SetWindowStateAndWait(ctx, tconn, bw.ID, ash.WindowStateNormal); err != nil {
		s.Fatal("Failed to set Chrome window state to \"Normal\": ", err)
	}

	// Test that the "Float" button changes the window state to be floated.
	if err := showMultitaskMenu(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to open Multitask Menu: ", err)
	}
	floatButton := nodewith.Name("Float").HasClass("MultitaskButton")
	if err := ui.DoDefault(floatButton)(ctx); err != nil {
		s.Fatal("Failed to click the Float button: ", err)
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == bw.ID && w.State == ash.WindowStateFloated && !w.IsAnimating
	}, &pollOpts); err != nil {
		s.Fatalf("Unexpected Chrome window state: got %s, want %s: %v", bw.State, ash.WindowStateFloated, err)
	}

	// Test that the "Exit float" button restores the window state.
	unfloatButton := nodewith.Name("Exit float")
	if err := ui.DoDefault(unfloatButton)(ctx); err != nil {
		s.Fatal("Failed to click the Unfloat button: ", err)
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == bw.ID && w.State == ash.WindowStateNormal && !w.IsAnimating
	}, &pollOpts); err != nil {
		s.Fatalf("Unexpected Chrome window state: got %s, want %s: %v", bw.State, ash.WindowStateNormal, err)
	}

	// Test that the left "Half" button snaps the window to the left.
	if err := showMultitaskMenu(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to open Multitask Menu: ", err)
	}
	leftHalfButton := nodewith.NameContaining("left half")
	if err := ui.DoDefault(leftHalfButton)(ctx); err != nil {
		s.Fatal("Failed to click the Left Half button: ", err)
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == bw.ID && w.State == ash.WindowStatePrimarySnapped && !w.IsAnimating
	}, &pollOpts); err != nil {
		s.Fatalf("Unexpected Chrome window state: got %s, want %s: %v", bw.State, ash.WindowStatePrimarySnapped, err)
	}

	// Test that the right "Half" button snaps the window to the right.
	if err := showMultitaskMenu(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to open Multitask Menu: ", err)
	}
	rightHalfButton := nodewith.NameContaining("right half")
	if err := ui.DoDefault(rightHalfButton)(ctx); err != nil {
		s.Fatal("Failed to click the Right Half button: ", err)
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == bw.ID && w.State == ash.WindowStateSecondarySnapped && !w.IsAnimating
	}, &pollOpts); err != nil {
		s.Fatalf("Unexpected Chrome window state: got %s, want %s: %v", bw.State, ash.WindowStateSecondarySnapped, err)
	}
}

// showMultitaskMenu hovers over the Maximize button to show the multitask menu.
func showMultitaskMenu(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	maximizeButton := nodewith.Name("Maximize").HasClass("FrameSizeButton")
	maximizeButtonLoc, err := ui.Location(ctx, maximizeButton)
	if err != nil {
		return errors.Wrap(err, "failed to find maximize button bounds")
	}

	// Move the mouse into the maximize button with non-zero duration to hover.
	if err := mouse.Move(tconn, maximizeButtonLoc.CenterPoint(), 500*time.Millisecond)(ctx); err != nil {
		return errors.Wrap(err, "failed to move mouse onto the maximize button")
	}

	menu := nodewith.HasClass("MultitaskMenuBubbleWidget")
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(menu)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for multitask menu")
	}
	return nil
}
