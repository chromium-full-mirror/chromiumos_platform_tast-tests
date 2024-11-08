// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package versionutil provides utilities for querying Chrome's version.
package versionutil

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/chrome/version"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast/core/errors"
)

type lacrosMetadata struct {
	Content struct {
		Version string `json:"version"`
	} `json:"content"`
}

// AshVersion returns the version of Ash Chrome.
func AshVersion(ctx context.Context) (version.Version, error) {
	out, err := testexec.CommandContext(ctx, ashproc.ExecPath, "--version").Output(testexec.DumpLogOnError)
	if err != nil {
		return version.Version{}, err
	}
	versionStr := version.VersionRegexp.FindString(string(out))
	v := version.Parse(versionStr)
	if !v.IsValid() {
		return version.Version{}, errors.Errorf("invalid Chrome version: %v, from path: %v", versionStr, ashproc.ExecPath)
	}
	return v, nil
}
