// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arcent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/optin"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/testing"
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
	if err := a.EnableVerboseLogging(ctx, verboseTags...); err != nil {
		return err
	}
	return IncreaseLogcatBufferSize(ctx, a)
}

// IncreaseLogcatBufferSize increases the log buffer size to 10 MB.
func IncreaseLogcatBufferSize(ctx context.Context, a *arc.ARC) error {
	return a.Command(ctx, "logcat", "-G", "10M").Run(testexec.DumpLogOnError)
}

// WaitForInstallButton waits for Install button to show up on the app detail page.
func WaitForInstallButton(ctx context.Context, d *ui.Device) (*ui.Object, error) {
	const installButtonText = "install"
	installButton := d.Object(ui.ClassName("android.widget.Button"), ui.TextMatches("(?i)"+installButtonText))
	if err := installButton.WaitForExists(ctx, 10*time.Second); err != nil {
		return nil, err
	}
	return installButton, nil
}

// ValidateBlockedAppInstall validates that the blocked app is uninstalled automatically.
func ValidateBlockedAppInstall(ctx context.Context, a *arc.ARC, d *ui.Device, blockedPackage string) error {
	installButton, err := WaitForInstallButton(ctx, d)
	if err != nil {
		return errors.Wrap(err, "failed to find the install button")
	}

	enabled, err := installButton.IsEnabled(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check the install button state")
	}

	if !enabled {
		testing.ContextLog(ctx, "Install button is disabled")
		return nil
	}

	testing.ContextLog(ctx, "Install button is enabled. Attempting install")
	if err := installButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click the install button")
	}

	if err := a.WaitForPackages(ctx, []string{blockedPackage}); err != nil {
		// When the local view is cached and app shows as installable, Play Server rejects the
		// install request. If that happens, then the flow is validated.
		if err := d.Object(ui.TextMatches("(?i)Can't download .*")).Exists(ctx); err == nil {
			testing.ContextLog(ctx, "Blocked app not installable")
			return nil
		}

		return errors.Wrap(err, "package installation failed")
	}

	// If the install goes through, we expect it to be uninstalled immediately.
	testing.ContextLog(ctx, "Waiting for package to uninstall")
	if err := WaitForUninstall(ctx, a, blockedPackage); err != nil {
		return errors.Wrap(err, "package not uninstalled")
	}

	return nil
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
	const appUnavailableText = "Your administrator has not given you access to this item."

	obj := d.Object(ui.ClassName("android.widget.TextView"), ui.TextMatches("(?i)"+appUnavailableText))
	return obj.WaitForExists(ctx, timeout)
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
