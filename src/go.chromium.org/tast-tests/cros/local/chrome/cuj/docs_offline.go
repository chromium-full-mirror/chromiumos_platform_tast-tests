// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// EnsureDocsOfflineInstalled ensures that docs offline extension is installed.
func EnsureDocsOfflineInstalled(ctx context.Context, cr *chrome.Chrome) error {
	const (
		docsOfflineID   = "ghbmnnjooekpmoecnnnilnnbdlolhkhi"
		docsOfflineName = "Google Docs Offline"
		docsOfflineURL  = "https://chromewebstore.google.com/detail/google-docs-offline/ghbmnnjooekpmoecnnnilnnbdlolhkhi"
	)
	docsOfflineExt := cws.App{Name: docsOfflineName, URL: docsOfflineURL}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	isInstalled, err := ash.ExtensionAppInstalled(ctx, tconn, docsOfflineID)
	if err != nil {
		return errors.Wrap(err, "failed to check existence of docs offline extension")
	}

	if isInstalled {
		testing.ContextLog(ctx, "Docs offline extension is already installed")
		return nil
	}

	testing.ContextLog(ctx, "Install docs offline extension")
	// Allow at maximum 2 minutes to install the extension. This normally only takes several seconds.
	cwsCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cwsErr := cws.InstallApp(cwsCtx, cr, docsOfflineExt)
	if cwsErr != nil {
		// If Docs Offline extention is included in /usr/share/google-chrome/extensions/,
		// it will be installed by the Chrome automatically.
		// Check if the extension has been installed even if the CWS installation fails.
		isInstalled, err := ash.ExtensionAppInstalled(ctx, tconn, docsOfflineID)
		if err == nil && isInstalled {
			testing.ContextLog(ctx, "Docs offline extension has been installed even though the CWS installation returned an error: ", cwsErr)
			// Sometimes the app installation may fail and return without closing the page.
			// Make sure the webstore page is closed if it's open.
			targets, err := cr.FindTargets(ctx, chrome.MatchTargetURLPrefix(docsOfflineURL))
			if err != nil {
				testing.ContextLog(ctx, "Failed to find Docs Offline page: ", err)
			} else if len(targets) != 0 {
				if err = cr.CloseTarget(ctx, targets[0].TargetID); err != nil {
					return errors.Wrap(err, "failed to close the webstore page")
				}
			}
			return nil
		}
	}
	return cwsErr
}

// EnsureDocsOfflineEnabled ensures that docs offline extension is installed
// and the current active user has it enabled in Drive's settings.
// This function should be called before opening any docs if offline capability
// is desired.
func EnsureDocsOfflineEnabled(ctx context.Context, cr *chrome.Chrome) error {
	if err := EnsureDocsOfflineInstalled(ctx, cr); err != nil {
		return errors.Wrap(err, "failed to install Docs offline extension")
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return errors.New("failed to get the out directory to dump UI tree on failures")
	}

	const totalRetry = 3
	retryNumber := 0
	ensureOfflineCheckboxChecked := func(ctx context.Context) (retErr error) {
		retryNumber++

		// Shorten context to allow for cleanup website resources and
		// dumping UI tree in case of failure.
		closeCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
		defer cancel()

		sctx, cancel := ctxutil.Shorten(ctx, time.Minute)
		defer cancel()
		// Open Drive settings page.
		conn, err := cr.NewConn(sctx, "https://drive.google.com/settings")
		if err != nil {
			return errors.Wrap(err, "failed to open Drive settings")
		}
		defer conn.Close()
		defer conn.CloseTarget(closeCtx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create Test API connection")
		}

		defer func(ctx context.Context) {
			if retryNumber == totalRetry {
				faillog.DumpUITreeWithScreenshotWithTestAPIOnError(ctx, outDir, func() bool { return retErr != nil }, tconn, "docs_offline_dump")
			}
		}(closeCtx)

		ui := uiauto.New(tconn)
		addAnotherAccountDialog := nodewith.HasClass("Widget").Role(role.Dialog)
		addAnotherAccountHeading := nodewith.NameContaining("Add another Google Account for").Role(role.Heading).Ancestor(addAnotherAccountDialog)
		closeButton := nodewith.Name("Close").Role(role.Button).HasClass("ImageButton").Ancestor(addAnotherAccountDialog)
		googleDriveRootWebArea := nodewith.Name("Settings - Google Drive").Role(role.RootWebArea)
		// It was found that during Lacros testing, the "Add another Google Account" dialog
		// might pop up. Dismiss the dialog before checking the offline checkbox.
		if err := uiauto.NamedCombine("dismiss 'Add another Google Account' dialog",
			uiauto.IfSuccessThen(ui.Exists(addAnotherAccountHeading), ui.LeftClick(closeButton)),
			uiauto.NamedAction("check if the page redirected to Drive Settings", ui.WaitUntilExists(googleDriveRootWebArea)),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure the Drive Settings page exist")
		}

		// Wait for settings page to load and sync the account settings.
		if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for Drive settings to load")
		}

		// Make sure the "offline" checkbox is checked.
		testing.ContextLog(ctx, "Making sure offline support is enabled")
		if err := conn.Call(ctx, nil, `() => {
				let offlineCheckbox = document.getElementsByName('offline')[0];
				if (!offlineCheckbox.checked)
					offlineCheckbox.click();
			}`); err != nil {
			return errors.Wrap(err, "failed to ensure offline checkbox checked")
		}
		return nil
	}

	if err := uiauto.Retry(totalRetry, ensureOfflineCheckboxChecked)(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure offline checkbox checked")
	}

	testing.ContextLog(ctx, "Docs offline support enabled")
	return nil
}
