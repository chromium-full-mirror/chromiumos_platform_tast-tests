// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"fmt"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/screenshot"
	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast/core/testing"
)

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
