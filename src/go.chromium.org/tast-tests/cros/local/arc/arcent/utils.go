// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arcent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/accountmanager"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// EnsurePackagesUninstall verifies that packages have desired uninstall behavior.
func EnsurePackagesUninstall(ctx context.Context, cr *chrome.Chrome, a *arc.ARC, packages []string, shouldUninstall bool) error {
	assertUninstall := func(isUninstalled bool, packageName string) error {

		action := "cannot"
		if isUninstalled {
			action = "can"
		}

		message := fmt.Sprintf("Package %q %s be uninstalled", packageName, action)

		if isUninstalled == shouldUninstall {
			testing.ContextLog(ctx, message)
			return nil
		}
		return errors.New(message)
	}

	testing.ContextLog(ctx, "Trying to uninstall packages")
	for _, p := range packages {
		err := a.Uninstall(ctx, p)
		isUninstalled := err == nil
		if err := assertUninstall(isUninstalled, p); err != nil {
			return err
		}
	}

	return nil
}

// WaitForUninstall waits for package to uninstall.
func WaitForUninstall(ctx context.Context, a *arc.ARC, blockedPackage string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if installed, err := a.PackageInstalled(ctx, blockedPackage); err != nil {
			return testing.PollBreak(err)
		} else if installed {
			return errors.New("Package not yet uninstalled")
		}
		return nil
	}, &testing.PollOptions{Interval: 1 * time.Second})
}

// DumpBugReportOnError dumps bug report on error.
func DumpBugReportOnError(ctx context.Context, hasError func() bool, a *arc.ARC, filePath string) {
	if !hasError() {
		return
	}

	testing.ContextLog(ctx, "Dumping Bug Report")
	if err := a.BugReport(ctx, filePath); err != nil {
		testing.ContextLog(ctx, "Failed to get bug report: ", err)
	}
}

// ConfigureProvisioningLogs enables verbose logging for important modules and increases the log buffer size.
func ConfigureProvisioningLogs(ctx context.Context, a *arc.ARC) error {
	verboseTags := []string{"clouddpc", "Finsky", "Volley", "PlayCommon"}
	return a.EnableVerboseLogging(ctx, verboseTags...)
}

// ValidateDisabledAppLaunch validates that a disabled app cannot be launched.
func ValidateDisabledAppLaunch(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device, disabledPackage, mainActivity string) error {
	const (
		disabledAppResourceID  = "com.android.settings:id/admin_support_dialog_title"
		disabledAppTextPattern = "(?i)Can.t open this app"
	)

	uia := uiauto.New(tconn)

	// Sometimes "am start" command doesn't return an error and the app doesn't launch either.
	return uia.Retry(2, func(ctx context.Context) error {
		testing.ContextLog(ctx, "Launching the app")
		if _, err := a.Command(ctx, "am", "start", "-n", fmt.Sprintf("%s/%s", disabledPackage, mainActivity)).Output(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to create main activity")
		}

		testing.ContextLog(ctx, "Waiting for the app disabled message to show")
		if err := d.Object(ui.ResourceID(disabledAppResourceID), ui.TextMatches(disabledAppTextPattern)).WaitForExists(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "launch error not found")
		}

		return nil
	})(ctx)
}

// ValidateBlockedAppInstall validates that the blocked app cannot be installed or is uninstalled automatically if installed.
func ValidateBlockedAppInstall(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device, blockedPackage string, timeout time.Duration) error {
	testing.ContextLog(ctx, "Validating install attempt for a blocked app")
	return PollAppPageState(ctx, tconn, a, blockedPackage, func(ctx context.Context) error {
		if err := playstore.FindAndDismissErrorDialog(ctx, d); err != nil {
			return errors.Wrap(err, "failed to dismiss server error dialog")
		}

		if err := WaitForAppUnavailableMessage(ctx, d, 10*time.Second); err == nil {
			testing.ContextLog(ctx, "App unavailable message found")
			return nil
		}

		installButton, err := playstore.FindInstallButton(ctx, d, 15*time.Second)
		if err != nil {
			return errors.Wrap(err, "failed to find the install button")
		}

		testing.ContextLog(ctx, "Install button is enabled. Attempting install")
		if err := installButton.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click the install button")
		}

		// Leave time for download failure check.
		installCtx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()
		if err := a.WaitForPackages(installCtx, []string{blockedPackage}); err != nil {
			// When the local view is cached and app shows as installable, Play Server rejects the
			// install request. If that happens, then the flow is validated.
			if err := d.Object(ui.TextMatches("(?i)Can.t download .*")).Exists(ctx); err == nil {
				testing.ContextLog(ctx, "Blocked app not installable")
				return nil
			}

			return errors.Wrap(err, "package installation failed")
		}

		// If the install goes through, we expect it to be uninstalled immediately.
		testing.ContextLog(ctx, "Waiting for package to uninstall")
		if err := WaitForUninstall(ctx, a, blockedPackage); err != nil {
			return testing.PollBreak(errors.Wrap(err, "package not uninstalled"))
		}

		testing.ContextLog(ctx, "Blocked app uninstalled")
		return nil
	}, timeout)
}

// PollAppPageState polls the Play Store app detail page for desired state.
func PollAppPageState(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, testPackage string, assertFn func(ctx context.Context) error, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := playstore.OpenAppPage(ctx, a, testPackage); err != nil {
			return testing.PollBreak(err)
		}

		err := assertFn(ctx)

		if err != nil {
			testing.ContextLogf(ctx, "App page for %q not in desired state: %s", testPackage, err)
			playstore.Close(ctx, a)
		}
		return err
	}, &testing.PollOptions{Timeout: timeout, Interval: 30 * time.Second})
}

// WaitForAppUnavailableMessage waits for the message shown for blocked apps.
func WaitForAppUnavailableMessage(ctx context.Context, d *ui.Device, timeout time.Duration) error {
	const appUnavailableText = ".*(not given you access|app isn.t available).*"

	selectors := [][]ui.SelectorOption{
		{ui.ClassName("android.view.View"), ui.DescriptionMatches(appUnavailableText)},
		{ui.ClassName("android.widget.TextView"), ui.TextMatches(appUnavailableText)},
	}

	var lastErr error
	waitTime := time.Second * time.Duration((int)(timeout.Seconds())/len(selectors))
	for _, selector := range selectors {
		lastErr = d.Object(selector...).WaitForExists(ctx, waitTime)
		if lastErr == nil {
			break
		}
	}

	return lastErr
}

// WaitForProvisioning waits for provisioning to finish and dumps logcat if doesn't.
func WaitForProvisioning(ctx context.Context, a *arc.ARC, attempt int) error {
	// CloudDPC sign-in timeout set in code is 3 minutes.
	const provisioningTimeout = 3 * time.Minute

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		if err := optin.DumpLogCat(ctx, strconv.Itoa(attempt)); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to dump logcat: %s", err)
		}
		return err
	}
	return nil
}

// AddSecondaryAccount adds a secondary account to CrOS account manager.
func AddSecondaryAccount(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, d *ui.Device, user, pass string) error {
	ui := uiauto.New(tconn).WithTimeout(time.Minute)
	if err := uiauto.Combine("open account manager settings",
		accountmanager.OpenAccountManagerSettingsAction(tconn, cr),
		ui.LeftClickUntil(
			nodewith.Name("Add Google Account").Role(role.Button),
			ui.Exists(accountmanager.AddAccountDialog())),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open account manager settings")
	}

	if err := accountmanager.AddAccount(ctx, tconn, user, pass); err != nil {
		return errors.Wrap(err, "failed to add an account in account manager")
	}

	return nil
}
