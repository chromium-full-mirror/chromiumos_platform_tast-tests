// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const defaultUIDumpFileName = "arc_uidump"

// NewUIDevice creates a Device object by starting and connecting to UI Automator server.
// Close must be called to clean up resources when a test is over.
func (a *ARC) NewUIDevice(ctx context.Context) (*ui.Device, error) {
	return ui.NewDeviceWithRetry(ctx, a.device)
}

// DumpUIHierarchy dumps the arc UI hierarchy to 'arc_uidump.xml'. This
// function should be called after closing arc UI devices, since only one can be
// attached at a time or it will fail with code 137.
func (a *ARC) DumpUIHierarchy(ctx context.Context, outDir string) error {
	return a.DumpUIHierarchyToFile(ctx, outDir, defaultUIDumpFileName)
}

// DumpUIHierarchyToFile dumps the arc UI hierarchy to a file with the given
// base file name (without extension). This function should be called after
// closing arc UI devices, since only one can be attached at a time or it will
// fail with code 137.
func (a *ARC) DumpUIHierarchyToFile(ctx context.Context, outDir, fileName string) error {
	dumpFile := "/sdcard/window_dump.xml"

	if err := a.Command(ctx, "uiautomator", "dump").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to dump arc UI")
	}
	defer a.Command(ctx, "rm", dumpFile).Run(testexec.DumpLogOnError)

	dir := filepath.Join(outDir, "faillog")
	if err := os.MkdirAll(dir, 0777); err != nil {
		return errors.Wrapf(err, "failed to create directory %s", dir)
	}
	path := filepath.Join(dir, fileName+".xml")
	if err := a.PullFile(ctx, dumpFile, path); err != nil {
		return errors.Wrap(err, "failed to pull UI dump to outDir")
	}

	testing.ContextLogf(ctx, "Test failed. Dumped ARC UI hierarchy into %q", path)
	return nil
}

// DumpUIHierarchyOnError dumps arc UI hierarchy to 'arc_uidump.xml' when the
// test fails. Call this function after closing arc UI devices, since only
// one can be attached at a time or it will fail with code 137.
func (a *ARC) DumpUIHierarchyOnError(ctx context.Context, outDir string, hasError func() bool) error {
	if !hasError() {
		return nil
	}

	return a.DumpUIHierarchy(ctx, outDir)
}

// DumpUIHierarchyHandler dumps the arc UI hierarchy to 'arc_uidump.xml'. A UI
// device is optional. Pass nil if no devices are open. If provided, closes the
// given UI device since only one instance can be attached at a time.
//
// This function can be used with s.AttachErrorHandlers to dump the hierarchy
// immediately after failure, before any cleanup functions run. For example:
//
// uiDumpHandler := a.DumpUIHierarchyHandler(cleanupCtx, d, s.OutDir())
//
// s.AttachErrorHandlers(uiDumpHandler, uiDumpHandler)
func (a *ARC) DumpUIHierarchyHandler(ctx context.Context, d *ui.Device, outDir string) func(string) {
	return a.DumpUIHierarchyToFileHandler(ctx, d, outDir, defaultUIDumpFileName)
}

// DumpUIHierarchyToFileHandler dumps the arc UI hierarchy to a file with the
// given base file name (without extension). A UI device is optional. Pass nil
// if no devices are open. If provided, closes the given UI device since only
// one instance can be attached at a time.
//
// This function can be used with s.AttachErrorHandlers to dump the hierarchy
// immediately after failure, before any cleanup functions run. For example:
//
// uiDumpHandler := a.DumpUIHierarchyToFileHandler(cleanupCtx, d, s.OutDir())
//
// s.AttachErrorHandlers(uiDumpHandler, uiDumpHandler)
func (a *ARC) DumpUIHierarchyToFileHandler(ctx context.Context, d *ui.Device, outDir, fname string) func(string) {
	return func(_ string) {
		if d != nil {
			testing.ContextLog(ctx, "Closing existing UI device to dump UI hierarchy")
			if err := d.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close ARC UI device: ", err)
			}
		}
		if err := a.DumpUIHierarchyToFile(ctx, outDir, fname); err != nil {
			testing.ContextLog(ctx, "Failed to dump ARC UI hierarchy: ", err)
		}
	}
}

// OpenPlayStoreAccountSettings opens account settings in PlayStore where user
// can switch between available accounts.
func OpenPlayStoreAccountSettings(ctx context.Context, arcDevice *androidui.Device, tconn *chrome.TestConn) error {
	// Ensure the Play Store is closed so that we don't resume another session.
	if err := apps.Close(ctx, tconn, apps.PlayStore.ID); err != nil {
		testing.ContextLog(ctx, "Failed to close Play Store: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		return errors.Wrap(err, "failed to launch Play Store")
	}

	noThanksButton := arcDevice.Object(androidui.ClassName("android.widget.Button"),
		androidui.TextMatches("(?i)No thanks"))
	if err := noThanksButton.WaitForExists(ctx, 10*time.Second); err != nil {
		testing.ContextLog(ctx, "No Thanks button doesn't exist: ", err)
	} else if err := noThanksButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click No Thanks button")
	}

	avatarIcon := arcDevice.Object(androidui.ClassName("android.widget.FrameLayout"),
		androidui.DescriptionContains("Account and settings"))
	if err := avatarIcon.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Avatar Icon")
	}

	expandAccountButton := arcDevice.Object(androidui.DescriptionContains("Expand account"), androidui.Enabled(true))
	if err := expandAccountButton.WaitForExists(ctx, 10*time.Second); err != nil {
		testing.ContextLog(ctx, "Expand account button doesn't exist: ", err)
	} else if err := expandAccountButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click expand account button")
	}
	return nil
}

// SwitchPlayStoreAccount switches between the ARC account in PlayStore.
func SwitchPlayStoreAccount(ctx context.Context, arcDevice *androidui.Device, tconn *chrome.TestConn, accountEmail string) error {
	accountNameButton := arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.Text(accountEmail))

	if err := uiauto.Retry(3, func(ctx context.Context) error {
		// The added new account sometimes need more time to be reflected in Play Store settings.
		if err := OpenPlayStoreAccountSettings(ctx, arcDevice, tconn); err != nil {
			return errors.Wrap(err, "failed to open Play Store account settings")
		}

		if err := accountNameButton.WaitForExists(ctx, 30*time.Second); err != nil {
			return errors.Wrap(err, "failed to find Account Name")
		}
		return nil
	})(ctx); err != nil {
		return errors.Wrap(err, "failed to open Play Store account settings and find account name")
	}

	if err := accountNameButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Account Name")
	}

	return nil
}

// ClickAddAccountInSettings clicks Add account > Google in ARC Settings. Settings window should be already open.
func ClickAddAccountInSettings(ctx context.Context, arcDevice *androidui.Device, tconn *chrome.TestConn) error {
	const (
		scrollClassName   = "android.widget.ScrollView"
		textViewClassName = "android.widget.TextView"
	)
	addAccount := arcDevice.Object(androidui.ClassName("android.widget.TextView"),
		androidui.TextMatches("(?i)Add account"), androidui.Enabled(true))

	if err := addAccount.Exists(ctx); err != nil {
		// Scroll until Accounts is visible.
		scrollLayout := arcDevice.Object(androidui.ClassName(scrollClassName),
			androidui.Scrollable(true))
		accounts := arcDevice.Object(androidui.ClassName("android.widget.TextView"),
			androidui.TextMatches("(?i).*Accounts"), androidui.Enabled(true))
		if err := scrollLayout.WaitForExists(ctx, 10*time.Second); err == nil {
			scrollLayout.ScrollTo(ctx, accounts)
		}
		if err := accounts.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click on Accounts")
		}
		if err := addAccount.WaitForExists(ctx, 10*time.Second); err != nil {
			return errors.Wrap(err, "failed finding Add account")
		}
	}

	if err := addAccount.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Add account")
	}

	accountTypeButton := func(text string) *androidui.Object {
		return arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)"+text), androidui.Enabled(true), androidui.ResourceIDMatches("(android:id/title)"))
	}

	// Click on account type if the account type screen is shown e.g. Google, Exchange, Managed, etc.
	if err := accountTypeButton("Exchange|Managed Account").WaitForExists(ctx, 10*time.Second); err != nil {
		testing.ContextLog(ctx, "Google button doesn't exist: ", err)
	} else if err := accountTypeButton("Google").Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Google account type button")
	} else {
		testing.ContextLog(ctx, "Google account type button found and clicked")
	}
	return nil
}
