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

const (
	modernPinFeatureName         = "CrOSLateBootEnableModernPin"
	migrateToModenPinFeatureName = "CrOSLateBootMigrateToModernPin"
	pinweaverPasswordFeatureName = "CrOSLateBootPinweaverForPassword"
)

// WithModernPin executes a block of code after enabling the ModernPin feature.
func WithModernPin(ctx context.Context, f func() error) error {
	enableFeatureOption := chrome.EnableFeatures(modernPinFeatureName)
	disableFeatureOption := chrome.DisableFeatures(migrateToModenPinFeatureName)
	cr, err := chrome.New(ctx, chrome.DeferLogin(), enableFeatureOption, disableFeatureOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome at the login screen")
	}
	defer cr.Close(ctx)
	return f()
}

// WithModernPinDisabled executes a block of code after disabling the ModernPin feature.
func WithModernPinDisabled(ctx context.Context, f func() error) error {
	featureOption := chrome.DisableFeatures(modernPinFeatureName, migrateToModenPinFeatureName)
	cr, err := chrome.New(ctx, chrome.DeferLogin(), featureOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome at the login screen")
	}
	defer cr.Close(ctx)
	return f()
}

// WithMigrationPin executes a code block after enabling the migration of pins.
func WithMigrationPin(ctx context.Context, f func() error) error {
	featureOption := chrome.EnableFeatures(migrateToModenPinFeatureName, modernPinFeatureName)
	cr, err := chrome.New(ctx, chrome.DeferLogin(), featureOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome at the login screen")
	}
	defer cr.Close(ctx)
	return f()
}

// WithPinweaverPasswords executes a code block after enabling the migration of pins.
func WithPinweaverPasswords(ctx context.Context, f func() error) error {
	featureOption := chrome.EnableFeatures(pinweaverPasswordFeatureName)
	cr, err := chrome.New(ctx, chrome.DeferLogin(), featureOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome at the login screen")
	}
	defer cr.Close(ctx)
	return f()
}
