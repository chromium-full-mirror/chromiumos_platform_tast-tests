// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/bundles/cros/wmp/wmputils"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/mouse"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/pointer"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FloatWindowMultitaskMenu,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test that the float multitask menu works",
		Contacts: []string{
			"chromeos-wm-corexp@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > FloatingWindow
		BugComponent: "b:1252568",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Val: browser.TypeAsh,
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 2*time.Minute,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

// FloatWindowMultitaskMenu tests floating a window via the Multitask Menu.
func FloatWindowMultitaskMenu(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(),
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
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
	defer pc.Close()

	ui := uiauto.New(tconn)
	const timeout = 30 * time.Second
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Test that the "Float" button changes the window state to be floated.
	if err := showMultitaskMenu(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to open Multitask Menu: ", err)
	}
	floatButton := nodewith.Name("Float").HasClass("MultitaskButton")
	if err := pc.Click(floatButton)(ctx); err != nil {
		s.Fatal("Failed to click the Float button: ", err)
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == bw.ID && w.State == ash.WindowStateFloated && !w.IsAnimating
	}, &testing.PollOptions{Timeout: timeout, Interval: time.Second}); err != nil {
		s.Fatalf("Unexpected Chrome window state: got %s, want %s", bw.State, ash.WindowStateFloated)
	}

	// Test that the "Exit float" button restores the window state.
	unfloatButton := nodewith.Name("Exit float")
	if err := pc.Click(unfloatButton)(ctx); err != nil {
		s.Fatal("Failed to click the Unfloat button: ", err)
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == bw.ID && w.State == ash.WindowStateNormal && !w.IsAnimating
	}, &testing.PollOptions{Timeout: timeout, Interval: time.Second}); err != nil {
		s.Fatalf("Unexpected Chrome window state: got %s, want %s", bw.State, ash.WindowStateNormal)
	}
}

// showMultitaskMenu hovers over the Maximize button to show the multitask menu.
func showMultitaskMenu(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	maximizeButton := nodewith.Name("Maximize").HasClass("FrameSizeButton")
	maximizeButtonLoc, err := ui.Location(ctx, maximizeButton)
	if err != nil {
		return errors.Wrap(err, "failed to find maximize button bounds")
	}

	if err := mouse.Move(tconn, maximizeButtonLoc.CenterPoint(), 0)(ctx); err != nil {
		return errors.Wrap(err, "failed to move mouse onto the maximize button")
	}

	menu := nodewith.HasClass("MultitaskMenuBubbleWidget")
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(menu)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for multitask menu")
	}
	return nil
}
