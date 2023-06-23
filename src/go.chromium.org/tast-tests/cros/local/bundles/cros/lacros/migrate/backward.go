// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package migrate contains functionality shared by tests that tests profile
// migration from Ash to Lacros.
package migrate

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/localstate"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// BackwardRun starts Chrome and waits until Lacros to Ash migration is marked as completed by Ash.
// Once the migration is completed, it will relaunch Ash Chrome and return the new `chrome.Chrome` instance.
// Callers are expected to add the Chrome options and set up the environment to trigger backward migration.
func BackwardRun(ctx context.Context, chromeOpts []chrome.Option) (*chrome.Chrome, error) {
	// TODO(chromium:1290297): This is a hack.
	// chrome.New doesn't really support profile migration because it
	// doesn't anticipate the additional Chrome restart that profile
	// migration effects. As a result, the *Chrome return value is already
	// invalid and we must not use it. Moreover, we must disable the
	// RemoveNotification option because otherwise chrome.New will try to
	// interact with Chrome at a time when that is no longer safe.
	// In order to obtain a valid *Chrome value for the test to continue
	// with, we restart Chrome once more after profile migration.
	testing.ContextLog(ctx, "Restarting for profile migration")
	chromeOpts = append(chromeOpts,
		chrome.KeepState(),
		chrome.RemoveNotification(false),
		chrome.DisableFeatures("LacrosOnly"),
	)

	crDoNotUse, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if crDoNotUse != nil {
			crDoNotUse.Close(ctx)
		}
	}()

	userHash, err := cryptohome.UserHash(ctx, crDoNotUse.Creds().User)
	if err != nil {
		return nil, err
	}
	testing.ContextLog(ctx, "Waiting for backward profile migration to complete for ", userHash)
	pref := "lacros.profile_data_backward_migration_completed_for_user." + userHash
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		completedVal, err := localstate.UnmarshalPref(browser.TypeAsh, pref)
		if err != nil {
			return err
		}
		completed, ok := completedVal.(bool)
		if !ok || !completed {
			return errors.New("backward profile migration incomplete")
		}
		return nil
	}, nil); err != nil {
		return nil, err
	}

	crDoNotUse.Close(ctx)
	crDoNotUse = nil
	testing.ContextLog(ctx, "Restarting after backward profile migration")
	return chrome.New(ctx, chromeOpts...)
}
