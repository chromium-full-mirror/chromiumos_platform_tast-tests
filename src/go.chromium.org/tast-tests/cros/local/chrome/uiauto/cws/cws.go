// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cws provides a utility to install apps from the Chrome Web Store.
package cws

import (
	"context"
	"net/url"
	"path"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// InstallationTimeout defines the maximum time duration to install an app from the Chrome Web Store.
const InstallationTimeout = 5 * time.Minute

// App contains info about a Chrome Web Store app. All fields are required.
type App struct {
	Name string // Name of the Chrome app.
	URL  string // URL to install the app from.
}

// pollOpts is the polling interval and timeout to be used on the Chrome Web Store.
var pollOpts = &testing.PollOptions{Interval: time.Second, Timeout: InstallationTimeout}

// InstallAppWithTimeout installs the specified Chrome app from Chrome Web Store with timeout.
func InstallAppWithTimeout(ctx context.Context, cr *chrome.Chrome, app App, timeout time.Duration) error {
	installCtx, cancel := context.WithDeadline(ctx, time.Now().Add(timeout))
	defer cancel()
	return InstallApp(installCtx, cr, app)
}

// InstallApp installs the specified Chrome app from the Chrome Web Store.
func InstallApp(ctx context.Context, cr *chrome.Chrome, app App) (retErr error) {
	cws, err := cr.NewConn(ctx, app.URL)
	if err != nil {
		return err
	}
	defer cws.Close()
	defer cws.CloseTarget(ctx)

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return errors.New("failed to get the out directory to dump UI tree on failures")
	}

	// Shorten context to allow for dumping UI tree in case of failure.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(closeCtx, outDir, func() bool { return retErr != nil }, tconn, "install_app_dump")

	var (
		add     = nodewith.Role(role.Button).Name(`Add to Chrome`).First()
		confirm = nodewith.Role(role.Button).NameRegex(regexp.MustCompile(`Add (app|extension)`))
		emailRE = regexp.MustCompile(`^[-.+\w]+@([-.+\w]+?\.)+[-.+\w]+$`)
		account = nodewith.Role(role.PopUpButton).NameRegex(emailRE)
		// User account restricts for the new (dogfood) CWS page.
		newAccountRE      = regexp.MustCompile(`^Google Account:[^\(]+\([-.+\w]+@([-.+\w]+?\.)+[-.+\w]+\)`)
		newAccount        = nodewith.Role(role.Button).NameRegex(newAccountRE)
		closeDialogButton = nodewith.Role(role.Button).Name("Close dialog")
	)
	ui := uiauto.New(tconn)

	// Helper to wait for the account element on the  current CWS page or the new
	// dogfood CWS page.
	waitForAccount := uiauto.NamedCombine("wait for account",
		ui.WaitUntilAnyExists(closeDialogButton, account, newAccount),
		uiauto.IfSuccessThen(
			ui.Exists(closeDialogButton),
			ui.DoDefault(closeDialogButton),
		),
		ui.WaitUntilAnyExists(account, newAccount),
	)

	// Check if the account has been added to Chrome Web Store page.
	// There might be a timing issue that the account has been added to Lacros profile but not yet propagated to the web page
	// due to different ways of looking up the credentials. Browser uses OAuth, the web reads cookies instead.
	// If this happens, it fails to load the app page with the account. See crbug.com/1322246 for details.
	// To get around it for recovery it gives a retry by reloading the page to the app page URL.
	// TODO(crbug.com/1375314): Figure out how to avoid this timing issue in product, rather than in tests.
	if err := waitForAccount(ctx); err != nil {
		if err := browser.ReloadActiveTab(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to reload page")
		}
		if err := cws.Navigate(ctx, app.URL); err != nil {
			return errors.Wrapf(err, "failed to navigate page: %v", app.URL)
		}
		if err := waitForAccount(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for account to be added")
		}
	}
	// Retrieving the ID of the app out from the URL.
	u, err := url.Parse(app.URL)
	if err != nil {
		return err
	}
	// The ID should be the last element of path.
	appID := path.Base(u.Path)

	// Click the add button at most once to prevent triggering
	// weird UI behaviors in Chrome Web Store.
	addClicked := false
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Check if the app is installed.
		if installed, err := ash.ExtensionAppInstalled(ctx, tconn, appID); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to check if app is installed"))
		} else if installed {
			return nil
		}

		if !addClicked {
			// If the app is not installed, install it now.
			// Click on the add button, if it exists.
			if err := ui.Exists(add)(ctx); err == nil {
				if err := ui.DoDefault(add)(ctx); err != nil {
					return testing.PollBreak(err)
				}
				addClicked = true
			}
		}

		// Click on the confirm button, if it exists.
		if err := uiauto.IfSuccessThen(ui.Exists(confirm), ui.LeftClick(confirm))(ctx); err != nil {
			return testing.PollBreak(err)
		}

		// The newly installed extension can potentially open new tabs.
		// Activating cws target should switch back to the installation page.
		if err := cws.ActivateTarget(ctx); err != nil {
			return testing.PollBreak(err)
		}

		return errors.Errorf("%s still installing", app.Name)
	}, pollOpts); err != nil {
		return errors.Wrapf(err, "failed to install %s", app.Name)
	}
	return nil
}

// UninstallApp uninstalls the specified Chrome app from the Chrome Web Store.
func UninstallApp(ctx context.Context, cr *chrome.Chrome, app App) error {
	cws, err := cr.NewConn(ctx, app.URL)
	if err != nil {
		return err
	}
	defer cws.Close()
	defer cws.CloseTarget(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	ui := uiauto.New(tconn)
	return uiauto.Combine("uninstall the extension from CWS",
		ui.DoDefault(nodewith.Role(role.Button).Name("Remove from Chrome").First()),
		ui.LeftClick(nodewith.Role(role.Button).Name("Remove")),
	)(ctx)
}

// Screencastify is one of the popular extensions for screen video recorder.
var Screencastify = App{
	Name: "Screencastify",
	URL:  "https://chrome.google.com/webstore/detail/screencastify-screen-vide/mmeijimgabbpbgpdklnllpncmdofkcpn",
}

// Kami extension for Google Chrome.
var Kami = App{
	Name: "Kami",
	URL:  "https://chrome.google.com/webstore/detail/kami-for-google-chrome/ecnphlgnajanjnkcmbpancdjoidceilk",
}

// ReadAndWrite represents the "Read&Write" extension for Google Chrome.
var ReadAndWrite = App{
	Name: "Read&Write",
	URL:  "https://chrome.google.com/webstore/detail/readwrite-for-google-chro/inoeonmfapjbbkmdafoankkfajkcphgd",
}
