// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/internal/extension"
	"go.chromium.org/tast/core/errors"
)

// AddTastLibrary introduces tast library into the page for the given conn.
// This introduces a variable named "tast" to its scope, and it is the
// caller's responsibility to avoid the conflict.
func AddTastLibrary(ctx context.Context, conn *Conn) error {
	// Ensure the page is loaded so the tast library will be added properly.
	if err := conn.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
		return errors.Wrap(err, "failed waiting for page to load")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := conn.WaitForExpr(checkCtx, extension.TastLibraryLoadedExpr); err == nil {
		return nil
	}
	return conn.Eval(ctx, extension.TastLibraryJS, nil)
}

// ExtensionBackgroundPageURL returns the URL to the background page for
// the extension with the supplied ID.
func ExtensionBackgroundPageURL(extID string) string {
	return extension.BackgroundPageURL(extID)
}

// ExtensionServiceWorkerURL returns the URL to the background page for
// the extension with the supplied ID.
func ExtensionServiceWorkerURL(extID, serviceWorker string) string {
	return extension.ServiceWorkerURL(extID, serviceWorker)
}
