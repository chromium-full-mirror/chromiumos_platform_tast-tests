// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package faillog

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
)

// DumpUITreeWithScreenshotHandler is an error handler to save UI tree and
// screenshot.
// It can be used with s.AttachErrorHandlers.
func DumpUITreeWithScreenshotHandler(ctx context.Context, tconn *chrome.TestConn, prefix string) func(errMsg string) {
	return func(errMsg string) {
		filename := fmt.Sprintf("%s-%s", prefix, time.Now().Format(time.RFC3339Nano))
		DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(ctx, func() bool { return true }, tconn, filename)
	}
}
