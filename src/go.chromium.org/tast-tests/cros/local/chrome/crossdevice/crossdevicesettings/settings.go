// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crossdevicesettings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// ConnectedDevicesSettingsRelativePath is the path to the connected device settings page.
	ConnectedDevicesSettingsRelativePath = "multidevice/features"
	// ConnectedDevicesSettingsURL is the full url to the connected device settings page.
	ConnectedDevicesSettingsURL = "chrome://os-settings/" + ConnectedDevicesSettingsRelativePath
	// MultidevicePageJS is the JS locator for the multidevice settings page.
	MultidevicePageJS = `shadowPiercingQuery("settings-multidevice-page")`
	// MultideviceSubpageJS is the JS locator for the multidevice settings subpage element.
	MultideviceSubpageJS = MultidevicePageJS + `.shadowRoot` +
		`.querySelector("settings-multidevice-subpage")`
	// ConnectedDeviceToggleVisibleJS is the JS locator for checking if the toggle to enable "Connected devices" is visible.
	ConnectedDeviceToggleVisibleJS = MultidevicePageJS + `.shouldShowToggle_()`
)

// OSSettingsWithShadowPiercer returns a Chrome conn to OS settings with the `shadowPiercingQuery` function loaded.
// This function enables element location that can look within nested shadow roots. All functions that interact
// with multidevice settings pages in OS Settings rely on `shadowPiercingQuery` to execute JS and drive the pages,
// so any interactions with the OS Settings through a Chrome conn should be done using this function.
func OSSettingsWithShadowPiercer(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, url string, existingConn bool) (*chrome.Conn, error) {
	launchURL := url
	if url == "" {
		launchURL = "chrome://os-settings/"
	}
	var conn *chrome.Conn
	var err error
	if existingConn {
		conn, err = cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix(launchURL))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start Chrome session to existing OS settings with URL %v", launchURL)
		}
	} else {
		// Close any existing settings windows before re-launching.
		shown, err := ash.AppShown(ctx, tconn, apps.Settings.ID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to check if OS Settings is open")
		}
		if shown {
			if err := apps.Close(ctx, tconn, apps.Settings.ID); err != nil {
				return nil, errors.Wrap(err, "failed to close already opened OS Settings instance")
			}
		}
		// Launching OS Settings pagees sometimes fails at the first time so we will retry a few times.
		if err := uiauto.Retry(5, func(ctx context.Context) error {
			conn, err = apps.LaunchOSSettings(ctx, cr, launchURL)
			if err != nil {
				return errors.Wrapf(err, "failed to start Chrome session to OS settings with URL %v", launchURL)
			}
			return nil
		})(ctx); err != nil {
			return nil, errors.Wrapf(err, "failed to start Chrome session to OS settings with URL %v after retries", launchURL)
		}

	}
	// Execute some arbitrary JS with `EvalWithShadowPiercer` to ensure `shadowPiercingQuery` is loaded.
	if err := webutil.EvalWithShadowPiercer(ctx, conn, "true", nil); err != nil {
		return nil, errors.Wrap(err, "failed to load shadow piercer")
	}
	return conn, nil
}

// WaitForConnectedDevice waits for the Android device to appear in the 'Connected device' section of OS Settings.
// Note: it can take up to 5 minutes for an Android device to successfully pair with the Chromebook.
// To account for this, the deadline of the passed in context should expire no sooner than 5 minutes from when this function is called.
func WaitForConnectedDevice(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) error {
	settingsConn, err := OSSettingsWithShadowPiercer(ctx, tconn, cr /*url=*/, "" /*existingConn=*/, false)
	if err != nil {
		return err
	}
	defer settingsConn.Close()

	// Use JS to wait for a phone to be connected. If we are stuck waiting for the
	// "Connected devices" toggle to become visible (i.e. waiting for verification)
	// for more than 5 minutes, attempt to force a sync through the debug page.
	if ctxutil.DeadlineBefore(ctx, time.Now().Add(5*time.Minute)) {
		d, _ := ctx.Deadline()
		t := d.Sub(time.Now())
		return errors.Errorf("insufficient time remaining before the context reaches its deadline. need at least 5 minutes, only %v remain", t)
	}
	testing.ContextLog(ctx, "Waiting up to 5 minutes for the devices to be paired")
	if err := settingsConn.WaitForExpr(ctx, MultidevicePageJS); err != nil {
		return errors.Wrap(err, "failed waiting for \"Connected devices\" subpage to load")
	}
	if err := settingsConn.WaitForExprWithTimeout(ctx, ConnectedDeviceToggleVisibleJS, 5*time.Minute); err != nil {
		// Force a sync with the chrome://proximity-auth sync button and keep waiting.
		testing.ContextLog(ctx, "Devices did not pair within 5 minutes. Attempting to force a sync through the debug page")
		conn, err := cr.NewConn(ctx, "chrome://proximity-auth")
		if err != nil {
			return errors.Wrap(err, "failed to open chrome://proximity-auth")
		}
		defer conn.Close()
		syncBtn := `document.getElementById("force-device-sync")`
		if err := conn.WaitForExpr(ctx, syncBtn); err != nil {
			return errors.Wrap(err, "failed waiting for chrome://proximity-auth 'Sync' button to load")
		}
		if err := conn.Eval(ctx, syncBtn+`.click()`, nil); err != nil {
			return errors.Wrap(err, "failed to click chrome://proximity-auth 'Sync' button")
		}
		if err := settingsConn.WaitForExpr(ctx, ConnectedDeviceToggleVisibleJS); err != nil {
			return errors.Wrap(err, "'Connected devices' subpage not available after forcing device sync")
		}
	}
	return nil
}
