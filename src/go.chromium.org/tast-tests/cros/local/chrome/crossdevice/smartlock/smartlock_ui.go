// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package smartlock is for controlling ChromeOS Smart Lock functionality.
package smartlock

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/crossdevicesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	multidevicePasswordPrompt = crossdevicesettings.MultidevicePageJS + `.shadowRoot.querySelector("settings-password-prompt-dialog")`
	smartLockToggle           = crossdevicesettings.MultideviceSubpageJS +
		`.shadowRoot.getElementById("smartLockItem")` +
		`.shadowRoot.querySelector("settings-multidevice-feature-toggle")` +
		`.shadowRoot.getElementById("toggle")`
	smartLockToggleChecked = smartLockToggle + `.checked`
)

// OpenConnectedDevicesPage opens the multidevice settings page in OS Settings
func OpenConnectedDevicesPage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*ossettings.OSSettings, error) {
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, filepath.Join(crossdevicesettings.ConnectedDevicesSettingsRelativePath), func(context.Context) error { return nil })
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch OS Settings at the multidevice feature page")
	}
	return settings, nil
}

// ToggleSmartLockEnabled opens the Connected devices page in OS Settings and toggles Smart Lock's enabled state
func ToggleSmartLockEnabled(ctx context.Context, enable bool, tconn *chrome.TestConn, cr *chrome.Chrome, password string) error {
	settings, err := OpenConnectedDevicesPage(ctx, tconn, cr)
	if err != nil {
		return errors.Wrap(err, "failed to open Connected devices page in OS Settings")
	}
	settingsConn, err := crossdevicesettings.OSSettingsWithShadowPiercer(ctx, tconn, cr, crossdevicesettings.ConnectedDevicesSettingsURL /*existingConn=*/, true)
	if err != nil {
		return err
	}
	defer settingsConn.Close()

	if err := chrome.AddTastLibrary(ctx, settingsConn); err != nil {
		settingsConn.Close()
		return errors.Wrap(err, "failed to introduce tast library")
	}

	if err := settingsConn.WaitForExpr(ctx, smartLockToggle); err != nil {
		return errors.Wrap(err, "failed to find the Smart Lock toggle")
	}

	var toggleChecked bool
	if err := settingsConn.Eval(ctx, smartLockToggleChecked, &toggleChecked); err != nil {
		return errors.Wrap(err, "failed to read Smart Lock toggle checked property")
	}
	if toggleChecked == enable {
		// Smart Lock is already in the desired state.
		return nil
	}

	// Flip the toggle. If toggling on, we need to provide an auth token.
	if enable {
		token, err := settings.AuthToken(ctx, settingsConn, password)
		if err != nil {
			return errors.Wrap(err, "failed to get auth token")
		}
		data, err := json.Marshal(token)
		if err != nil {
			return errors.Wrap(err, "failed to marshal auth token to JSON")
		}
		expr := fmt.Sprintf(`%s.authToken_ = %s`, crossdevicesettings.MultidevicePageJS, data)
		if err := settingsConn.Eval(ctx, expr, nil); err != nil {
			return errors.Wrap(err, "failed to set authToken_ property")
		}
		if err := settingsConn.Eval(ctx, smartLockToggle+`.click()`, nil); err != nil {
			return errors.Wrap(err, "failed to toggle Smart Lock enabled on")
		}
		// When the toggle is enabled, the password dialog will be shown,
		// but we only need to cancel it since we've already provided a token.
		if err := settingsConn.Eval(ctx, multidevicePasswordPrompt+`.onCancelClick_()`, nil); err != nil {
			return errors.Wrap(err, "failed to close password prompt")
		}
	} else {
		if err := settingsConn.Eval(ctx, smartLockToggle+`.click()`, nil); err != nil {
			return errors.Wrap(err, "failed to toggle Smart Lock enabled off")
		}
	}

	// Ensure that the Smart Lock toggle is now in the desired state. Wait
	// up to 3 seconds since the change doesn't take effect instantaneously.
	var expr string = smartLockToggleChecked
	if !enable {
		expr = `!` + expr
	}
	if err := settingsConn.WaitForExprWithTimeout(ctx, expr, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to change Smart Lock enabled state")
	}
	return nil
}

// SignOut ends the existing chrome session and logs out by keyboard shortcut.
func SignOut(ctx context.Context, cr *chrome.Chrome, kb *input.KeyboardEventWriter) error {
	cr.Close(ctx)
	if err := kb.Accel(ctx, "Ctrl+Shift+Q"); err != nil {
		return errors.Wrap(err, "failed to emulate shortcut 1st press")
	}
	if err := kb.Accel(ctx, "Ctrl+Shift+Q"); err != nil {
		return errors.Wrap(err, "failed to emulate shortcut 2nd press")
	}
	return nil
}

// CheckSmartLockVisibilityOnSigninScreen signs out and checks whether
// Smart Lock is visible on the signin screen, before signing back in.
func CheckSmartLockVisibilityOnSigninScreen(ctx context.Context, expectVisible bool, cr *chrome.Chrome, tconn *chrome.TestConn, loginOpts, noLoginOpts []chrome.Option) (*chrome.Chrome, *chrome.TestConn, error) {
	// Logout is done by keyboard shortcut.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	// Sign out
	if cr, tconn, err = goToLoginScreen(ctx, cr, tconn, kb, loginOpts, noLoginOpts); err != nil {
		return nil, nil, err
	}

	if err = lockscreen.WaitForSmartLockVisible(ctx, expectVisible, tconn); err != nil {
		return nil, nil, err
	}

	// Sign in
	if cr, tconn, err = signInWithPassword(ctx, cr, tconn, loginOpts); err != nil {
		return nil, nil, errors.Wrap(err, "failed to sign in")
	}

	return cr, tconn, nil
}

// CheckSmartLockVisibilityOnLockScreen locks the session and checks whether
// Smart Lock is visible on the lock screen, before unlocking.
func CheckSmartLockVisibilityOnLockScreen(ctx context.Context, expectVisible bool, tconn *chrome.TestConn, username, password string) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	if err := lockscreen.Lock(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to lock the screen on ChromeOS")
	}

	if err = lockscreen.WaitForSmartLockVisible(ctx, expectVisible, tconn); err != nil {
		return err
	}

	// Unlock
	if err := lockscreen.EnterPassword(ctx, tconn, username, password, kb); err != nil {
		return errors.Wrap(err, "failed to unlock with password")
	}

	return nil
}

// goToLoginScreen signs out of the current session a couple of times so that
// signin screen settings have a chance to take effect.
func goToLoginScreen(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, loginOpts, noLoginOpts []chrome.Option) (*chrome.Chrome, *chrome.TestConn, error) {
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		return nil, nil, errors.Wrap(err, "failed to sleep before SignOut")
	}

	var err error
	if err = SignOut(ctx, cr, kb); err != nil {
		return nil, nil, errors.Wrap(err, "failed to sign out")
	}

	cr, err = chrome.New(
		ctx,
		noLoginOpts...,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to start chrome")
	}
	tconn, err = cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting API connection failed")
	}

	return cr, tconn, nil
}

func signInWithPassword(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, loginOpts []chrome.Option) (*chrome.Chrome, *chrome.TestConn, error) {
	cr, err := chrome.New(
		ctx,
		loginOpts...,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to login to Chrome")
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "creating test API connection failed")
	}

	return cr, tconn, nil
}
