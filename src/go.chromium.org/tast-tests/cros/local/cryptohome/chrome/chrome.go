// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chrome contains cryptohome-specific chrome utilities.
package chrome

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// WithModernPin executes a block of code after enabling the ModernPin feature.
func WithModernPin(ctx context.Context, f func() error) error {
	const featureName = "CrOSLateBootEnableModernPin"
	featureOption := chrome.EnableFeatures(featureName)
	cr, err := chrome.New(ctx, chrome.DeferLogin(), featureOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome at the login screen")
	}
	defer cr.Close(ctx)
	return f()
}

// WithModernPinDisabled executes a block of code after disabling the ModernPin feature.
func WithModernPinDisabled(ctx context.Context, f func() error) error {
	const featureName = "CrOSLateBootEnableModernPin"
	featureOption := chrome.DisableFeatures(featureName)
	cr, err := chrome.New(ctx, chrome.DeferLogin(), featureOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome at the login screen")
	}
	defer cr.Close(ctx)
	return f()
}

// WithMigrationPin executes a code block after enabling the migration of pins.
func WithMigrationPin(ctx context.Context, f func() error) error {
	const migrationFeatureName = "CrOSLateBootMigrateToModernPin"
	const modernFeatureName = "CrOSLateBootEnableModernPin"
	featureOption := chrome.EnableFeatures(migrationFeatureName, modernFeatureName)
	cr, err := chrome.New(ctx, chrome.DeferLogin(), featureOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome at the login screen")
	}
	defer cr.Close(ctx)
	return f()
}
