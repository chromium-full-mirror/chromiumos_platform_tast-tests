// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
)

// doFullScreenAction returns an action that does the given fullScreenAction.
func doFullScreenAction(tconn *chrome.TestConn, fullScreenAction action.Action, title string, isFullScreen bool) action.Action {
	ui := uiauto.New(tconn)
	actionDescription := "enter full screen"
	if !isFullScreen {
		actionDescription = "exit full screen"
	}
	return uiauto.NamedAction(actionDescription,
		ui.Retry(3, uiauto.Combine(actionDescription,
			fullScreenAction,
			ash.WaitForFullscreenConditionWithTitle(tconn, title, isFullScreen, 10*time.Second),
		)))
}

// takeScreenshot returns an action which captures a fullscreen screenshot.
func takeScreenshot(cr *chrome.Chrome, outDir, name string) action.Action {
	return func(ctx context.Context) error {
		path := fmt.Sprintf("%s/screenshot-%s.png", outDir, name)
		if err := screenshot.CaptureChrome(ctx, cr, path); err != nil {
			testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
		}
		return nil
	}
}
