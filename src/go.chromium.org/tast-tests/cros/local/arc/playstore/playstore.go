// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package playstore provides set of util functions used to install applications through the playstore.
package playstore

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// operation indicates a specific operation that can be performed in the Play Store.
type operation string

const (
	// Install an app from the Play Store, wait for it to complete and verify the installation.
	installApp operation = "install"
	// Begin installing an app from the Play Store. Does not wait for, or verify, the installation.
	beginAppInstall operation = "beginAppInstall"
	// Update an app from the Play Store and wait for it to complete.
	updateApp        operation = "update"
	playStorePackage string    = "com.android.vending"
)

// Options contains options used when installing or updating an app.
type Options struct {
	// TryLimit limits number of tries to install or update an app.
	// Default value is 3, and -1 means unlimited.
	TryLimit int

	// DefaultUITimeout is used when waiting for UI elements.
	// Default value is 20 sec.
	DefaultUITimeout time.Duration

	// ShortUITimeout is used when waiting for "Complete account setup" button.
	// Default value is 10 sec.
	ShortUITimeout time.Duration

	// InstallationTimeout is used when waiting for app installation.
	// Default value is 90 sec.
	InstallationTimeout time.Duration
}

// FindInstallButton finds the install button on app detail page.
func FindInstallButton(ctx context.Context, d *ui.Device, timeout time.Duration) (*ui.Object, error) {
	return FindActionButton(ctx, d, "install", timeout)
}

// FindActionButton finds the action button on app detail page.
func FindActionButton(ctx context.Context, d *ui.Device, actionText string, timeout time.Duration) (*ui.Object, error) {
	var result *ui.Object

	buttonClass := ui.ClassName("android.widget.Button")
	actionButton := d.Object(buttonClass, ui.TextMatches("(?i)"+actionText), ui.Enabled(true))

	viewClass := ui.ClassName("android.view.View")
	actionView := d.Object(viewClass, ui.DescriptionMatches("(?i)"+actionText), ui.Enabled(true))

	textViewClass := ui.ClassName("android.widget.TextView")
	actionTextView := d.Object(textViewClass, ui.DescriptionMatches("(?i)"+actionText), ui.Enabled(true))
	actionTextView2 := d.Object(textViewClass, ui.TextMatches("(?i)"+actionText), ui.Enabled(true))

	result, err := findAnyExists(ctx, timeout, actionTextView2, actionTextView, actionView, actionButton)
	if err != nil {
		return nil, errors.Wrapf(err, "could not find the button %q", actionText)
	}
	return result, nil
}

// findAnyExists finds and returns the first object that exists.
func findAnyExists(ctx context.Context, timeout time.Duration, objects ...*ui.Object) (*ui.Object, error) {
	var targetObject *ui.Object
	err := testing.Poll(ctx, func(ctx context.Context) error {
		for _, object := range objects {
			if err := object.Exists(ctx); err == nil {
				targetObject = object
				return nil
			}
		}
		return errors.New("failed to find any given objects")
	}, &testing.PollOptions{Timeout: timeout, Interval: time.Second})

	return targetObject, err
}

// FindAndDismissErrorDialog finds and dismisses all possible intermittent errors in Play Store.
func FindAndDismissErrorDialog(ctx context.Context, d *ui.Device) error {
	const (
		serverErrorText           = "Server busy.*|Server error|Error.*server.*|.*connection with the server.|Connection timed out."
		cantDownloadText          = "Can.t download.*"
		cantInstallText           = "Can.t install.*"
		compatibleText            = "Your device is not compatible with this item."
		openMyAppsText            = "Please open my apps.*"
		termsOfServiceText        = "Terms of Service"
		installAppsFromDeviceText = "Install apps from your devices"
		internalProblemText       = "There.s an internal problem with your device.*"
		itemNotFoundText          = ".*item.*could not be found.*"
		discountOfPlayPassText    = "Get .* off Play Pass"
		trialOfPlayPassText       = "Get a 1-month trial of Google Play Pass"
		tryPlayPassText           = "Try Google Play Pass"
		stayInLoopText            = "Want to stay in the loop.*"

		acceptButtonText       = "accept"
		gotItButtonText        = "got it"
		okButtonText           = "ok"
		noThanksButtonText     = "No thanks"
		tryAgainOrOkButtonText = "Try again|OK"
		notNowButtonText       = "Not now"
	)

	for _, val := range []struct {
		dialogText string
		buttonText string
	}{
		// Due to timing of propagation of policy, the UI may be enabled but the item is not available.
		{itemNotFoundText, okButtonText},
		// These are intermittent server side errors that can happen under load.
		{serverErrorText, tryAgainOrOkButtonText},
		// Sometimes a dialog of "Can't download <app name>" pops up. Press "Got it" to
		// dismiss the dialog. This check needs to be done before checking the
		// install button since the install button exists underneath.
		{cantDownloadText, gotItButtonText},
		// Similarly, press "Got it" button if "Can't install <app name>" dialog pops up.
		{cantInstallText, gotItButtonText},
		// Also, press Ok to dismiss the dialog if "Please open my apps" dialog pops up.
		{openMyAppsText, okButtonText},
		// Also, press "NO THANKS" to dismiss the dialog if "Install apps from your devices" dialog pops up.
		{installAppsFromDeviceText, noThanksButtonText},
		// When Play Store hits the rate limit it sometimes show "Your device is not compatible with this item." error.
		// This error is incorrect and should be ignored like the "Can't download <app name>" error.
		{compatibleText, okButtonText},
		// Somehow, playstore shows a ToS dialog upon opening even after playsore
		// optin finishes. Click "accept" button to accept and dismiss.
		{termsOfServiceText, acceptButtonText},
		// Press Ok to dismiss the dialog if "There\'s an internal problem with your device" dialog pops up.
		{internalProblemText, okButtonText},
		// Press "No thanks" to dismiss if the dialog of "Get X% off Play Pass" pops up.
		{discountOfPlayPassText, noThanksButtonText},
		// Press "Not now" to dismiss if the dialog of "Get a 1-month trial of Google Play Pass" pops up.
		{trialOfPlayPassText, notNowButtonText},
		// Press "Not now" to dismiss if the dialog of "Try Google Play Pass" pops up.
		{tryPlayPassText, notNowButtonText},
		// Press "Not now" to dismiss if the dialog of "Want to stay in the loop?" pops up.
		{stayInLoopText, notNowButtonText},
	} {
		if err := FindAndDismissDialog(ctx, d, val.dialogText, val.buttonText); err != nil {
			return err
		}
	}

	return nil
}

// FindAndDismissDialog finds a dialog containing text with a corresponding button and presses the button.
func FindAndDismissDialog(ctx context.Context, d *ui.Device, dialogText, buttonText string) error {
	if err := d.Object(ui.TextMatches("(?i)" + dialogText)).Exists(ctx); err == nil {
		testing.ContextLogf(ctx, `%q popup found. Skipping`, dialogText)
		okButton := d.Object(ui.ClassName("android.widget.Button"), ui.TextMatches("(?i)"+buttonText))
		if err := okButton.WaitForExists(ctx, time.Second); err != nil {
			return err
		}
		if err := okButton.Click(ctx); err != nil {
			return err
		}
	}
	return nil
}

// OpenAppPage opens the detail page of an app in Play Store.
func OpenAppPage(ctx context.Context, a *arc.ARC, pkgName string) error {
	const (
		intentActionView    = "android.intent.action.VIEW"
		playStoreAppPageURI = "market://details?id="
	)

	if err := a.SendIntentCommand(ctx, intentActionView, playStoreAppPageURI+pkgName).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to send intent to open the Play Store")
	}

	return nil
}

// Close closes Play Store.
func Close(ctx context.Context, a *arc.ARC) error {
	return a.Command(ctx, "am", "force-stop", playStorePackage).Run(testexec.DumpLogOnError)
}

// printPercentageOfAppInstalled func prints the percentage of app installed so far.
func printPercentageOfAppInstalled(ctx context.Context, d *ui.Device) {
	const (
		currentInstallPercentInGBText = ".*GB"
		currentInstallPercentInMBText = ".*MB"
		currentPerInfoClassName       = "android.widget.TextView"
	)
	for _, val := range []struct {
		currentPercentInfoClassName string
		currentInstallPercentInText string
	}{
		{currentPerInfoClassName, currentInstallPercentInMBText},
		{currentPerInfoClassName, currentInstallPercentInGBText},
	} {
		currPerInfo := d.Object(ui.ClassName(val.currentPercentInfoClassName), ui.TextMatches("(?i)"+val.currentInstallPercentInText))
		if err := currPerInfo.WaitForExists(ctx, time.Second); err == nil {
			getInfo, err := currPerInfo.GetText(ctx)
			if err == nil {
				testing.ContextLogf(ctx, "Percentage of app installed so far: %v ", getInfo)
			}
		}
	}
}

// installOrUpdate uses the Play Store to install or update an application.
func installOrUpdate(ctx context.Context, a *arc.ARC, d *ui.Device, pkgName string, opt *Options, op operation) error {
	const (
		accountSetupText      = "Complete account setup"
		permissionsText       = "needs access to"
		versionText           = "Your device isn.t compatible with this version."
		incompatibleText      = "This Chromebook isn't compatible with this app."
		linkPaypalAccountText = "Want to link your PayPal account.*"

		acceptButtonText    = "accept"
		continueButtonText  = "continue"
		installButtonText   = "install"
		uninstallButtonText = "uninstall"
		updateButtonText    = "update"
		openButtonText      = "open"
		playButtonText      = "play"
		retryButtonText     = "retry"
		tryAgainButtonText  = "try again"
		skipButtonText      = "skip"
		noThanksButtonText  = "No thanks"
	)

	o := *opt
	tryLimit := 3
	if o.TryLimit != 0 {
		tryLimit = o.TryLimit
	}
	defaultUITimeout := 20 * time.Second
	if o.DefaultUITimeout != 0 {
		defaultUITimeout = o.DefaultUITimeout
	}
	shortUITimeout := 10 * time.Second
	if o.ShortUITimeout != 0 {
		shortUITimeout = o.ShortUITimeout
	}
	installationTimeout := 90 * time.Second
	if o.InstallationTimeout != 0 {
		installationTimeout = o.InstallationTimeout
	}
	testing.ContextLogf(ctx, "Using TryLimit=%d, DefaultUITimeout=%s, ShortUITimeout=%s, InstallationTimeout=%s",
		tryLimit, defaultUITimeout, shortUITimeout, installationTimeout)

	testing.ContextLog(ctx, "Opening Play Store with Intent")
	if err := a.WaitIntentHelper(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for ArcIntentHelper")
	}

	if err := OpenAppPage(ctx, a, pkgName); err != nil {
		return err
	}

	var btnText string // Action button text - install or update.
	switch op {
	case installApp, beginAppInstall:
		// Look for install button.
		btnText = installButtonText
	case updateApp:
		// Look for update button.
		btnText = updateButtonText
	default:
		return errors.Errorf("operation %s is not supported", op)
	}

	// Wait for the app to install or update.
	testing.ContextLogf(ctx, "Waiting for app to %s", op)

	tries := 0

	retriesExhausted := func() bool {
		return tryLimit != -1 && tries >= tryLimit
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := FindAndDismissErrorDialog(ctx, d); err != nil {
			return testing.PollBreak(err)
		}

		// If the version isn't compatible with the device, no install button will be available.
		// Fail immediately.
		versionTextView := d.Object(ui.TextMatches(versionText))
		versionDescView := d.Object(ui.DescriptionMatches(versionText))
		incompatibleTextView := d.Object(ui.TextMatches(incompatibleText))
		if _, err := findAnyExists(ctx, defaultUITimeout, versionTextView, versionDescView, incompatibleTextView); err == nil {
			return testing.PollBreak(errors.New("device not compatible with this version or this app"))
		}

		// If retry button appears, reopen the Play Store page by sending the same intent again.
		// (It tends to work better than clicking the retry button.)
		if _, err := FindActionButton(ctx, d, fmt.Sprintf("(?i)(%s|%s)", retryButtonText, tryAgainButtonText), defaultUITimeout); err == nil {
			if retriesExhausted() {
				return testing.PollBreak(errors.Errorf("reopen Play Store attempt limit of %d times", tryLimit))
			}

			tries++
			testing.ContextLogf(ctx, "Retry button is shown. Trying to reopen the Play Store. Total attempts so far: %d", tries)
			if err := OpenAppPage(ctx, a, pkgName); err != nil {
				return err
			}
		}

		// If play or open button exists, the app is already installed or still installing.
		// Skip searching for install or update button to avoid clicking remote-install buttons.
		openOrPlayStr := fmt.Sprintf("(%s|%s)", openButtonText, playButtonText)
		if _, err := FindActionButton(ctx, d, openOrPlayStr, defaultUITimeout); err == nil {
			testing.ContextLog(ctx, "App is likely installing. Skipping Install button search")
			// If the install or update button is enabled, click it.
		} else if opButton, err := FindActionButton(ctx, d, btnText, 5*time.Second); err == nil {
			if retriesExhausted() {
				return testing.PollBreak(errors.Errorf("hit %s attempt limit of %d times", op, tryLimit))
			}

			tries++
			testing.ContextLogf(ctx, "Trying to hit the %s button. Total attempts so far: %d", op, tries)
			if err := opButton.Click(ctx); err != nil {
				return err
			}
		} else {
			testing.ContextLog(ctx, "Skipped clicking button to start operation: ", err)
		}

		// Grant permissions if necessary.
		if err := FindAndDismissDialog(ctx, d, permissionsText, acceptButtonText); err != nil {
			return testing.PollBreak(err)
		}

		// Handle "Want to link your PayPal account" if necessary.
		testing.ContextLogf(ctx, "Checking existence of : %s", linkPaypalAccountText)
		if err := d.Object(ui.TextMatches("(?i)"+linkPaypalAccountText), ui.Enabled(true)).WaitForExists(ctx, 10*time.Second); err == nil {
			testing.ContextLog(ctx, "Want to link your paypal account does exist")
			if noThanksButton, err := FindActionButton(ctx, d, noThanksButtonText, defaultUITimeout); err != nil {
				return testing.PollBreak(err)
			} else if err := noThanksButton.Click(ctx); err != nil {
				return testing.PollBreak(err)
			}
			if skipButton, err := FindActionButton(ctx, d, skipButtonText, defaultUITimeout); err != nil {
				return testing.PollBreak(err)
			} else if err := skipButton.Click(ctx); err != nil {
				return testing.PollBreak(err)
			}
		}

		// Complete account setup if necessary.
		testing.ContextLogf(ctx, "Checking existence of : %s", accountSetupText)
		if err := d.Object(ui.Text(accountSetupText), ui.Enabled(true)).WaitForExists(ctx, 10*time.Second); err == nil {
			testing.ContextLog(ctx, "Completing account setup")
			if continueButton, err := FindActionButton(ctx, d, continueButtonText, defaultUITimeout); err != nil {
				return testing.PollBreak(err)
			} else if err := continueButton.Click(ctx); err != nil {
				return testing.PollBreak(err)
			}

			if skipButton, err := FindActionButton(ctx, d, skipButtonText, defaultUITimeout); err != nil {
				return testing.PollBreak(err)
			} else if err := skipButton.Click(ctx); err != nil {
				return testing.PollBreak(err)
			}
		}

		// Grant permissions if necessary.
		if err := FindAndDismissDialog(ctx, d, permissionsText, acceptButtonText); err != nil {
			return testing.PollBreak(err)
		}

		// Do not wait for the install to complete in a beginAppInstall operation.
		if op == beginAppInstall {
			return nil
		}

		testing.ContextLog(ctx, "Checking existence of installation")
		// There are two possible of descriptions on the Play Store installation page.
		// One is "Download in progress", the other is "Install in progress".
		// If one of them exists, that means the installation is still in progress.
		progressDescription := d.Object(ui.DescriptionContains("in progress"))
		progressBar := d.Object(ui.ClassName("android.widget.ProgressBar"))
		progress, err := findAnyExists(ctx, shortUITimeout, progressDescription, progressBar)
		if err == nil {
			// Print the percentage of app installed so far.
			printPercentageOfAppInstalled(ctx, d)
			testing.ContextLog(ctx, "Wait until download and install complete")
			if err := progress.WaitUntilGone(ctx, installationTimeout); err != nil {
				return errors.Wrap(err, "installation is still in progress")
			}
		}

		// Make sure we are still on the Play Store installation page by checking whether the "open" or "play" button exists.
		// If not, reopen the Play Store page by sending the same intent again.
		if _, err := FindActionButton(ctx, d, openOrPlayStr, defaultUITimeout); err != nil {
			testing.ContextLog(ctx, "App installation page disappeared; reopen it")
			if err := OpenAppPage(ctx, a, pkgName); err != nil {
				return err
			}
		}

		installed, err := a.PackageInstalled(ctx, pkgName)
		if err != nil {
			return errors.Wrap(err, "failed to check app installation status")
		}
		if !installed {
			return errors.New("app not yet installed")
		}

		// Make sure necessary contents are installed.
		// Some apps have a two-stage installation process where it downloads the barebone
		// app first and then the actual app content for it to work properly. Waiting for
		// the `Uninstall`button to appear means it has finished installing all of the
		// necessary content.
		testing.ContextLog(ctx, "Checking if the app content is installed")
		if _, err := FindActionButton(ctx, d, uninstallButtonText, defaultUITimeout); err != nil {
			return errors.New("additional app content is still installing")
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second})
}

// InstallApp uses the Play Store to install an application.
// It will wait for the app to finish installing before returning.
// Play Store should be open to the homepage before running this function.
func InstallApp(ctx context.Context, a *arc.ARC, d *ui.Device, pkgName string, opt *Options) error {
	installed, err := a.PackageInstalled(ctx, pkgName)
	if err != nil {
		return err
	}
	if installed {
		return nil
	}

	if err := installOrUpdate(ctx, a, d, pkgName, opt, installApp); err != nil {
		return err
	}

	// Ensure that the correct package is installed, just in case the Play Store ui changes again.
	installed, err = a.PackageInstalled(ctx, pkgName)
	if err != nil {
		return err
	}
	if !installed {
		return errors.Errorf("failed to install %s", pkgName)
	}
	return nil
}

// BeginAppInstall uses the Play Store to trigger installation of an application.
// It will NOT wait for the app to finish installing before returning.
// Play Store should be open to the homepage before running this function.
func BeginAppInstall(ctx context.Context, a *arc.ARC, d *ui.Device, pkgName string, opt *Options) error {
	return installOrUpdate(ctx, a, d, pkgName, opt, beginAppInstall)
}

// InstallOrUpdateAppAndClose installs or updates an application via Play Store, closes Play Store after installation.
// If the application is already installed, it updates the app if an update is available.
// It will wait for the app to finish installing/updating and closes Play Store before returning.
func InstallOrUpdateAppAndClose(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device, pkgName string, opt *Options) error {
	installed, err := a.PackageInstalled(ctx, pkgName)
	if err != nil {
		return err
	}

	var installOperation operation
	if installed {
		testing.ContextLog(ctx, "App has already been installed; check if an update is available")
		installOperation = updateApp
	} else {
		testing.ContextLog(ctx, "App is not installed yet; check if an installation is available")
		installOperation = installApp
	}

	if err := installOrUpdate(ctx, a, d, pkgName, opt, installOperation); err != nil {
		return err
	}
	return optin.ClosePlayStore(ctx, tconn)
}

// LaunchAssetBrowserActivity starts the activity that displays the available apps.
func LaunchAssetBrowserActivity(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC) (*arc.Activity, error) {
	const (
		assetBrowserActivity = "com.android.vending.AssetBrowserActivity"
	)

	testing.ContextLog(ctx, "Starting Asset Browser activity")
	act, err := arc.NewActivity(a, playStorePackage, assetBrowserActivity)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new activity")
	}
	if err := act.Start(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed starting Play Store")
	}

	return act, nil
}

// VerifyPlayStoreWindowPresent verifies that Play Store window shows up.
func VerifyPlayStoreWindowPresent(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) error {
	uia := uiauto.New(tconn)
	classNameRegexp := regexp.MustCompile(`^ExoShellSurface(-\d+)?$`)
	playStoreUI := nodewith.Name("Play Store").Role(role.Window).ClassNameRegex(classNameRegexp)
	if err := uia.WithTimeout(timeout).WaitUntilExists(playStoreUI)(ctx); err != nil {
		return errors.Wrap(err, "failed to see Play Store window")
	}
	return nil
}
