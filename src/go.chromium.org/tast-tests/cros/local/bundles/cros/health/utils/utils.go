// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils provides util functions for health tast.
package utils

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/errors"
)

// For mocking
var readFile = ioutil.ReadFile

// ReadStringFileWithLeadingSpaces reads a file and returns its content as
// string while keeping the leading spaces.
func ReadStringFileWithLeadingSpaces(fpath string) (string, error) {
	v, err := readFile(fpath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read file: %v", fpath)
	}
	return strings.TrimRight(string(v), " \n"), nil
}

// ReadStringFile reads a file and returns its content as string.
func ReadStringFile(fpath string) (string, error) {
	v, err := readFile(fpath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read file: %v", fpath)
	}
	return strings.Trim(string(v), " \n"), nil
}

// ReadOptionalStringFile returns nil if file not found.
func ReadOptionalStringFile(fpath string) (*string, error) {
	v, err := ReadStringFile(fpath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

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
	workaroundModels := []string{"storo360", "kohaku", "joxer", "quandiso360", "foob360"}
	if slices.Contains(workaroundModels, model) {
		return ash.EnsureTabletModeDisabledWithKeyboardEnabled(ctx)
	}
	return ash.EnsureTabletModeEnabled(ctx, tconn, false)
}
