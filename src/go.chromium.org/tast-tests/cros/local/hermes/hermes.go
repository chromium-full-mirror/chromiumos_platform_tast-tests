// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hermes provides D-Bus wrappers and utilities for Hermes.
// https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/hermes/README.md
package hermes

import (
	"context"
	"reflect"
	"time"

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
	if err := testing.Poll(ctx, func(ctx context.Context) (e error) {
		return waitForHermesIdleHelper(ctx)
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "Timed out while checking if Hermes is idle")
	}
	return nil
}

func waitForHermesIdleHelper(ctx context.Context) error {
	euiccPaths, err := GetEUICCPaths(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to get available EUICCs")
	}
	for _, euiccPath := range euiccPaths {
		obj, err := dbusutil.NewDBusObject(ctx, hermesconst.DBusHermesService, hermesconst.DBusHermesEuiccInterface, euiccPath)
		if err != nil {
			return errors.Wrap(err, "unable to get EUICC object")
		}
		if err := testing.Poll(ctx, func(ctx context.Context) (e error) {
			return CheckProperty(ctx, obj, hermesconst.EuiccPropertyProfileRefreshedAtLeastOnce, true)
		}, nil); err != nil {
			return errors.Wrap(err, "Timed out waiting for ProfilesRefreshedAtleastOnce==true")
		}
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
