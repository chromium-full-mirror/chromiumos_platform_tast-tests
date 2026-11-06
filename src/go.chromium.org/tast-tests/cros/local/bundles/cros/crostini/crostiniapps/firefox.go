// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crostiniapps contains shared code used for app tests.
package crostiniapps

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Firefox test page content. Contains a single input box which is focused upon launch for user input. The test page contains two input boxes to test tab behaviour.
const html = `<!DOCTYPE html>
<title>Crostini Firefox Input Test Page</title>
<textarea style="width: 100%" autofocus></textarea>
<br>
<input style="width: 100%" />
`

const (
	// Slower devices could take up to two minutes to start Firefox.
	firefoxStartupTimeout = 2 * time.Minute
	testPageTitle         = "Crostini Firefox Input Test Page"
	testPageName          = "test_page.html"
	firefoxPoliciesName   = "policies.json"
	firefoxPoliciesPath   = "/etc/firefox/policies/policies.json"
	firefoxPoliciesJSON   = `{"policies":{` +
		`"DisableTelemetry":true,` +
		`"OverrideFirstRunPage":"",` +
		`"SkipTermsOfUse":true,` +
		`"Preferences":{` +
		`"browser.tabs.warnOnClose":{"Value":false,"Status":"locked"},` +
		`"datareporting.policy.dataSubmissionPolicyBypassNotification":{"Value":true,"Status":"locked"},` +
		`"datareporting.policy.firstRunURL":{"Value":"","Status":"locked"}` +
		`}}}`
)

var (
	firefoxWindow  = nodewith.NameRegex(regexp.MustCompile(`.*Mozilla Firefox`)).Role(role.Window).First()
	testPageWindow = nodewith.NameContaining(testPageTitle).Role(role.Window).First()
)

// configureFirefoxForTestPage installs temporary enterprise policies (including
// locked Preferences) to suppress first-run "Firefox Privacy Notice" / ToU tabs
// and multi-tab close prompts so the test page opens as the sole active tab.
func configureFirefoxForTestPage(ctx context.Context, cont *vm.Container) error {
	if err := cont.WriteFile(ctx, firefoxPoliciesName, firefoxPoliciesJSON); err != nil {
		return errors.Wrap(err, "failed to write Firefox policies")
	}
	if err := cont.Command(ctx, "sudo", "mkdir", "-p", "/etc/firefox/policies").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create Firefox policies directory")
	}
	if err := cont.Command(ctx, "chmod", "0644", firefoxPoliciesName).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to set permissions on Firefox policies file")
	}
	if err := cont.Command(ctx, "sudo", "mv", firefoxPoliciesName, firefoxPoliciesPath).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to install Firefox policies")
	}
	return nil
}

// CleanupFirefoxTestPageFiles removes the temporary test page and Firefox
// enterprise policy files from the container. Callers should pass a reserved
// cleanup context (created via ctxutil.Shorten) so cleanup still succeeds if
// the main test context times out.
func CleanupFirefoxTestPageFiles(ctx context.Context, cont *vm.Container) error {
	var firstErr error
	// Container.RemoveAll executes "sudo rm -rf", so it removes root-owned /etc
	// files and ignores paths that were already moved or never created.
	for _, path := range []string{testPageName, firefoxPoliciesName, firefoxPoliciesPath} {
		if err := cont.RemoveAll(ctx, path); err != nil && firstErr == nil {
			firstErr = errors.Wrapf(err, "failed to remove file %s", path)
		}
	}
	return firstErr
}

// LaunchFirefoxWithTestPage creates a test webpage and launches it with the Firefox app.
func LaunchFirefoxWithTestPage(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, cont *vm.Container, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) error {
	// Create a file with test page.
	if err := cont.WriteFile(ctx, testPageName, html); err != nil {
		return err
	}
	if err := configureFirefoxForTestPage(ctx, cont); err != nil {
		return err
	}

	maximizeWindow := func(ctx context.Context) error {
		// Explicitly find Firefox's Crostini window rather than calling
		// ash.GetActiveWindow, which races with Wayland ExoShellSurface
		// activation and can maximize the Terminal window instead.
		ffWin, err := ash.WaitForAnyWindow(ctx, tconn, func(w *ash.Window) bool {
			return w.WindowType == ash.WindowTypeCrostini && strings.Contains(w.Title, "Mozilla Firefox") && w.IsVisible && w.CanFocus
		})
		if err != nil {
			return errors.Wrap(err, "failed to find visible Firefox window")
		}
		if err := ffWin.ActivateWindow(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to activate Firefox window")
		}
		if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
			return w.ID == ffWin.ID && w.IsActive
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			return errors.Wrap(err, "failed waiting for Firefox window to be active")
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, ffWin.ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to maximize Firefox window")
		}
		return nil
	}

	if err := uiauto.Combine("create test page in terminal and open in firefox",
		terminalApp.RunCommand(keyboard, "MOZ_CRASHREPORTER_DISABLE=1 firefox-esr "+testPageName),
		ui.WithTimeout(firefoxStartupTimeout).WaitUntilExists(firefoxWindow),
		maximizeWindow,
		// Wait until Firefox sets the Wayland window title from <title> once the
		// test page loads as the active tab.
		ui.WithTimeout(firefoxStartupTimeout).WaitUntilExists(testPageWindow),
	)(ctx); err != nil {
		return err
	}

	return nil
}

// CloseFirefoxTestPage closes the Firefox webpage using keyboard shortcuts and
// waits for the window to close.
func CloseFirefoxTestPage(ctx context.Context, ui *uiauto.Context, keyboard *input.KeyboardEventWriter) error {
	// Retry pressing ctrl+w until the Firefox window closes, in case multiple tabs are open.
	return ui.WithInterval(time.Second).RetryUntil(
		keyboard.AccelAction("ctrl+w"),
		ui.WithTimeout(3*time.Second).WaitUntilGone(firefoxWindow),
	)(ctx)
}

// VerifyFirefoxLaunchAndClose verifies that firefox window exists and firefox on shelf.
// Firefox must have been launched.
// It also closes firefox.
func VerifyFirefoxLaunchAndClose(ctx context.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter) error {
	const startupTimeout = 2 * time.Minute // slower devices could take up to two minutes to start Firefox.
	// Use a 15s per-branch timeout rather than the 60s default so fallback
	// branches do not stall for 2 minutes when first-run tabs are suppressed.
	ud := uidetection.NewDefault(tconn).WithTimeout(15 * time.Second).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
	ui := uiauto.New(tconn).WithTimeout(startupTimeout)
	firefoxWindow := nodewith.NameRegex(regexp.MustCompile(`.*Mozilla Firefox`)).Role(role.Window).First()

	// Check firefox window. Accept "New Tab" in addition to the first-run
	// welcome text in case first-run tabs were suppressed or already dismissed.
	if err := uiauto.Combine("verify Firefox",
		ui.WaitUntilExists(firefoxWindow),
		uiauto.IfFailThen(
			ud.WaitUntilExists(uidetection.TextBlock([]string{"Welcome", "to", "Firefox"}).WithinA11yNode(firefoxWindow).First()),
			uiauto.IfFailThen(
				ud.WaitUntilExists(uidetection.TextBlock([]string{"Get", "started"}).WithinA11yNode(firefoxWindow).First()),
				ud.WaitUntilExists(uidetection.TextBlock([]string{"New", "Tab"}).WithinA11yNode(firefoxWindow).First()),
			),
		),
	)(ctx); err != nil {
		return err
	}

	// Check firefox is displayed on shelf.
	shelfItems, err := ash.ShelfItems(ctx, tconn)
	found := false
	if err != nil {
		return errors.Wrap(err, "failed to get shelf items")
	}
	for _, shelfItem := range shelfItems {
		if shelfItem.Title == "Firefox ESR" {
			found = true
			break
		}
	}
	if !found {
		return errors.New("failed to find firefox on shelf")
	}

	// Close firefox.
	if err := ui.WithInterval(time.Second).RetryUntil(
		keyboard.AccelAction("ctrl+w"),
		ui.WithTimeout(3*time.Second).WaitUntilGone(firefoxWindow),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to close firefox")
	}

	return nil
}

// DisableFirefoxWayland forces Firefox to use X11.
func DisableFirefoxWayland(ctx context.Context, cont *vm.Container) error {
	if err := cont.Command(ctx, "sudo", "sed", "-i", "-e", "s/Exec=/Exec=env MOZ_ENABLE_WAYLAND=0 /", "/usr/share/applications/firefox-esr.desktop").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to adjust .desktop file for firefox-esr")
	}

	return nil
}

// DisableFirefoxCSD forces Firefox to use server-side decorations.
func DisableFirefoxCSD(ctx context.Context, cont *vm.Container) error {
	if err := cont.WriteFile(ctx, "titlebar.js", `pref("browser.tabs.inTitlebar", 0);`); err != nil {
		return errors.Wrap(err, "failed to write file")
	}

	if err := cont.Command(ctx, "sudo", "mv", "titlebar.js", "/etc/firefox-esr/").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to move file into place")
	}

	return nil
}
