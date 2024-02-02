// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hermes provides D-Bus wrappers and utilities for Hermes.
// https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/hermes/README.md
package hermes

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"reflect"
	"time"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// JobName is the name of the hermes process
	JobName = "hermes"
)

// WaitForHermesIdle waits for Chrome to refresh installed profiles before returning.
func WaitForHermesIdle(ctx context.Context, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return waitForHermesIdleHelper(ctx)
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "Timed out while checking if Hermes is idle")
	}
	return nil
}

// WaitForChromeESIMCache waits for Chrome's eSIM cache to exist
func WaitForChromeESIMCache(ctx context.Context, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := ioutil.ReadFile("/home/chronos/Local State")
		return err
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "unable to read Chrome eSIM cache (b/269175859)")
	}
	return nil
}

func waitForHermesIdleHelper(ctx context.Context) error {
	euiccPaths, err := GetEUICCPaths(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to get available EUICCs")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return ensureEUICCSRefreshed(euiccPaths)
	}, nil); err != nil {
		return errors.Wrap(err, "Timed out waiting for Chrome to refresh profiles")
	}
	return nil
}

func ensureEUICCSRefreshed(euiccPaths []dbus.ObjectPath) error {
	jsonBytes, err := ioutil.ReadFile("/home/chronos/Local State")
	if err != nil {
		return errors.Wrap(err, "unable to read Chrome eSIM cache (b/269175859)")
	}

	var localState map[string]interface{}
	err = json.Unmarshal(jsonBytes, &localState)
	if err != nil {
		return errors.Wrap(err, "unable to unmarshal Chrome state")
	}

	c, ok := localState["cros_esim"].(map[string]interface{})
	if !ok {
		return errors.Wrap(err, "unable to read Chrome eSIM cache")
	}
	refreshedEuiccs, ok := c["refreshed_euiccs"].([]interface{})
	if !ok {
		return errors.Wrap(err, "unable to read refreshed EUICCs")
	}
	if len(refreshedEuiccs) != len(euiccPaths) {
		return errors.Wrap(err, "profiles haven't been refreshed from EUICCs")
	}
	return nil
}

// CheckProperty reads a DBus property on a DBusObject. Returns an error if the value does not match the expected value
func CheckProperty(ctx context.Context, o *dbusutil.DBusObject, prop string, expected interface{}) error {
	var actual interface{}
	if err := o.Property(ctx, prop, &actual); err != nil {
		return errors.Wrap(err, "failed to check property")
	}
	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		return errors.Errorf("unexpected type for %s, got: %T, want: %T", prop, actual, expected)
	}
	if actual != expected {
		return errors.Errorf("unexpected %s, got: %v, want: %v", prop, actual, expected)
	}

	return nil
}

// CheckNumInstalledProfiles checks installed profiles count matches expected profiles count.
func CheckNumInstalledProfiles(ctx context.Context, euicc *EUICC, expected int) error {
	installedProfiles, err := euicc.InstalledProfiles(ctx, false)
	if err != nil {
		return errors.Wrap(err, "failed to get installed profiles")
	}
	if len(installedProfiles) != expected {
		return errors.Errorf("unexpected number of installed profiles, got: %d, want: %d", len(installedProfiles), expected)
	}
	return nil
}

// InstallProfile installs provided profile
func InstallProfile(ctx context.Context, euicc *EUICC, profile Profile) error {
	activationCode, err := profile.ActivationCode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get activation code")
	}
	confirmationCode := ""
	response := euicc.Call(ctx, hermesconst.EuiccMethodInstallProfileFromActivationCode, activationCode, confirmationCode)
	if response.Err != nil {
		return errors.Wrapf(response.Err, "failed to install profile with code: %s", activationCode)
	}
	if len(response.Body) != 1 {
		return errors.Errorf("InstallProfile resulted in incorrect response len: %d", len(response.Body))
	}
	return nil
}

// UninstallProfile uninstalls provided profile
func UninstallProfile(ctx context.Context, euicc *EUICC, profile Profile) error {
	if response := euicc.Call(ctx, hermesconst.EuiccMethodUninstallProfile, profile.DBusObject.ObjectPath()); response.Err != nil {
		return errors.Wrap(response.Err, "failed to uninstall profile")
	}
	return nil
}
