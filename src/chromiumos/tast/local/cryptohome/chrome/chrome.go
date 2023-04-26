// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chrome contains cryptohome-specific chrome utilities.
package chrome

import (
	"context"

	"chromiumos/tast/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// WithUssMigration will execute a given block of code while enabling or
// disabling the USS migration feature, handling the setup and teardown of
// Chrome automatically. The given block should take no parameters and
// should return nil on success and an error if one occurs.
func WithUssMigration(ctx context.Context, enabled bool, f func() error) error {
	const featureName = "CrOSLateBootMigrateToUserSecretStash"
	var featureOption chrome.Option
	if enabled {
		featureOption = chrome.EnableFeatures(featureName)
	} else {
		featureOption = chrome.DisableFeatures(featureName)
	}
	cr, err := chrome.New(ctx, chrome.DeferLogin(), featureOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome at the login screen")
	}
	defer cr.Close(ctx)
	return f()
}
