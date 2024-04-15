// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/colorcmp"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// ToolkitConfig is the parameters for the toolkit test.
type ToolkitConfig struct {
	Data    string
	Command []string
}

// Toolkit runs a test program using a specified GUI toolkit.
func Toolkit(ctx context.Context, scriptPath, outDir string, conf *ToolkitConfig, cr *chrome.Chrome, tconn *chrome.TestConn, guest vm.Guest) error {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := guest.PushFile(ctx, scriptPath, conf.Data); err != nil {
		return errors.Wrapf(err, "failed to push %v to container", conf.Data)
	}

	testing.ContextLog(ctx, "Running the demo")
	cmd := guest.Command(ctx, conf.Command...)
	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "failed to start %q", shutil.EscapeSlice(cmd.Args))
	}

	defer cmd.Wait(testexec.DumpLogOnError)
	defer cmd.Kill()

	defer func(ctx context.Context) {
		revert, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
		if err != nil {
			testing.ContextLog(ctx, "Failed to ensure clamshell mode")
			return
		}
		defer revert(ctx)

		ui := uiauto.New(tconn)
		closeButton := nodewith.Name("Close").Onscreen()
		appInShelf := nodewith.HasClass("ShelfAppButton::AppStatusIndicatorView").Ancestor(nodewith.HasClass("ShelfView"))
		if err := uiauto.Combine("close app",
			ui.RightClickUntil(appInShelf, ui.WithTimeout(time.Second).WaitUntilExists(closeButton)),
			ui.LeftClickUntil(closeButton, ui.WithTimeout(time.Second).WaitUntilGone(closeButton)),
		)(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close application: ", err)
		}
	}(cleanupCtx)

	ud := uidetection.NewDefault(tconn)
	if err := uiauto.IfSuccessThen(
		ud.Exists(uidetection.TextBlockFromSentence("Google Play Store isn't responding")),
		ud.LeftClick(uidetection.TextBlockFromSentence("Close App")),
	)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to check or close \"Google Play Store isn't responding\" dialog: ", err)
	}

	// The toolkit applications will render a window filled with a specific shade of blue to disambiguate the color channels and catch potential format/swizzling/endianness bugs.
	if err := MatchScreenshotDominantColor(ctx, cr, colorcmp.RGB(0x12, 0x78, 0xEF), filepath.Join(outDir, "screenshot.png")); err != nil {
		return errors.Wrap(err, "failed during screenshot check")
	}

	return nil
}
