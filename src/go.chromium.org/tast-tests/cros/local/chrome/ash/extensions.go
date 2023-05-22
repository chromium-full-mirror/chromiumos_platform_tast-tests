// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ash

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ExtensionInfo corresponds to the "Extension Info" defined in
// https://developer.chrome.com/docs/extensions/reference/management/#type-ExtensionInfo.
type ExtensionInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ExtensionApps returns all of the installed extension apps.
func ExtensionApps(ctx context.Context, tconn *chrome.TestConn) ([]*ExtensionInfo, error) {
	var s []*ExtensionInfo
	if err := tconn.Call(ctx, &s, "tast.promisify(chrome.management.getAll)"); err != nil {
		return nil, errors.Wrap(err, "failed to call get all installed extension apps")
	}
	return s, nil
}

// ExtensionAppInstalled checks if an extension app specified by appID is installed.
func ExtensionAppInstalled(ctx context.Context, tconn *chrome.TestConn, appID string) (bool, error) {
	installedApps, err := ExtensionApps(ctx, tconn)
	if err != nil {
		return false, errors.Wrap(err, "failed to get all installed extension Apps")
	}

	for _, app := range installedApps {
		if app.ID == appID {
			return true, nil
		}
	}
	return false, nil
}

// WaitForExtensionAppInstalled waits for the app specified by appID to appear in extension installed apps.
func WaitForExtensionAppInstalled(ctx context.Context, tconn *chrome.TestConn, appID string, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if installed, err := ExtensionAppInstalled(ctx, tconn, appID); err != nil {
			return testing.PollBreak(err)
		} else if !installed {
			return errors.New("failed to wait for installed extension app by id: " + appID)
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: uiPollingInterval})
}

// WaitForExtensionAppUninstalled waits for the app specified by appID to disappear from extension installed apps.
func WaitForExtensionAppUninstalled(ctx context.Context, tconn *chrome.TestConn, appID string, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if installed, err := ExtensionAppInstalled(ctx, tconn, appID); err != nil {
			return testing.PollBreak(err)
		} else if installed {
			return errors.New("failed to wait for uninstalled extension app by id: " + appID)
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: uiPollingInterval})
}
