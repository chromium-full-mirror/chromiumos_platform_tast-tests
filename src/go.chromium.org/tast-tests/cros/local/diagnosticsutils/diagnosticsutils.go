// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package diagnosticsutils provides util functions for health and diagnostics tast.
package diagnosticsutils

import (
	"context"
	"path"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/errors"
)

// For mocking
var getCrosConfig = crosconfig.Get

// GetCrosConfig returns the cros config from path. The dir and base name are
// extracted from the argument as the cros config path and name.
func GetCrosConfig(ctx context.Context, cpath string) (string, error) {
	v, err := getCrosConfig(ctx, path.Dir(cpath), path.Base(cpath))
	if err != nil {
		return "", errors.Wrapf(err, "failed to get cros config: %v", cpath)
	}
	return v, nil
}

// GetOptionalCrosConfig returns nil if the config cannot be found.
func GetOptionalCrosConfig(ctx context.Context, cpath string) (*string, error) {
	v, err := GetCrosConfig(ctx, cpath)
	if err != nil {
		var e *crosconfig.ErrNotFound
		if errors.As(err, &e) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

// IsCrosConfigTrue returns if the value of cros config is true.
func IsCrosConfigTrue(ctx context.Context, cpath string) (bool, error) {
	v, err := GetOptionalCrosConfig(ctx, cpath)
	if err != nil {
		return false, err
	}
	r := v != nil && *v == "true"
	return r, nil
}

// EnsureClamshellMode makes sure that the clamshell mode state is enabled, and
// returns a function which reverts back to the original state.
//
// Typically, this will be used like:
//
//	cleanup, err := EnsureClamshellMode(ctx, tconn)
//	if err != nil {
//	  s.Fatal("Failed to ensure in clamshell mode: ", err)
//	}
//	defer cleanup(ctx)
func EnsureClamshellMode(ctx context.Context, tconn *chrome.TestConn) (func(ctx context.Context) error, error) {
	model, err := GetCrosConfig(ctx, "/name")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get model name")
	}
	// The function ash.EnsureTabletModeEnabled doesn't work on some models.
	// Use ash.EnsureTabletModeDisabledWithKeyboardEnabled, which uses ectool to
	// control the behavior.
	// See b/365033439.
	workaroundModels := []string{"storo360", "kohaku", "joxer", "quandiso360", "foob360", "kracko360"}
	if slices.Contains(workaroundModels, model) {
		return ash.EnsureTabletModeDisabledWithKeyboardEnabled(ctx)
	}
	return ash.EnsureTabletModeEnabled(ctx, tconn, false)
}
