// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// InstallApp installs the app via play store and prints app version name.
func InstallApp(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, d *androidui.Device, pkgName string) error {
	if err := playstore.InstallOrUpdateAppAndClose(ctx, tconn, a, d, pkgName, &playstore.Options{TryLimit: -1}); err != nil {
		return errors.Wrapf(err, "failed to install %s", pkgName)
	}

	return logAppVersion(ctx, a, d, pkgName)
}

// InstallAppFromAPKURL installs the app from the given APK URL.
func InstallAppFromAPKURL(ctx context.Context, a *arc.ARC, d *androidui.Device, pkgName, apkURL string) error {
	// Uninstall the app if it is already installed
	// to ensure the app is installed from the given APK.
	if err := UninstallApp(ctx, a, pkgName); err != nil {
		return errors.Wrap(err, "failed to uninstall app before installing from APK URL")
	}

	apkContent, err := utils.FetchFromURL(ctx, apkURL)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch apk content from %s", apkURL)
	}

	apkPath := filepath.Join(os.TempDir(), path.Base(apkURL))
	if err := os.WriteFile(apkPath, []byte(apkContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write apk file")
	}
	defer os.Remove(apkPath)

	if err := a.Install(ctx, apkPath); err != nil {
		return errors.Wrapf(err, "failed to install %s from apk", pkgName)
	}
	return logAppVersion(ctx, a, d, pkgName)
}

// logAppVersion prints app version name.
func logAppVersion(ctx context.Context, a *arc.ARC, d *ui.Device, pkgName string) error {
	out, err := a.Command(ctx, "dumpsys", "package", pkgName).Output(testexec.DumpLogOnError)
	if err != nil {
		return err
	}

	output := string(out)
	splitOutput := strings.Split(output, "\n")
	const versionNamePrefix = "versionName="
	for splitLine := range splitOutput {
		if strings.Contains(splitOutput[splitLine], versionNamePrefix) {
			versionName := strings.Split(splitOutput[splitLine], "=")[1]
			testing.ContextLogf(ctx, "App version: %s", versionName)
			break
		}
	}
	return nil
}

// UninstallApp uninstalls the app if it has been installed.
func UninstallApp(ctx context.Context, a *arc.ARC, pkgName string) error {
	installed, err := a.PackageInstalled(ctx, pkgName)
	if err != nil {
		return errors.Wrap(err, "failed to get package install status")
	}
	if !installed {
		testing.ContextLog(ctx, "The app is already uninstalled")
		return nil
	}
	return a.Uninstall(ctx, pkgName)
}

// LaunchApp launches the app.
func LaunchApp(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, app apps.App) error {
	if err := launcher.SearchAndWaitForAppOpen(tconn, kb, app)(ctx); err != nil {
		return errors.Wrapf(err, "failed to launch %s app", app.Name)
	}
	return nil
}

// CloseApp closes app window.
func CloseApp(ctx context.Context, tconn *chrome.TestConn, pkgName string) error {
	w, err := ash.GetARCAppWindowInfo(ctx, tconn, pkgName)
	if err != nil {
		return errors.Wrap(err, "failed to get app window info")
	}
	return w.CloseWindow(ctx, tconn)
}
