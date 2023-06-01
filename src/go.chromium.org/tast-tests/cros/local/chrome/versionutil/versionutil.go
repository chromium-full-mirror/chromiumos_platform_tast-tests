// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package versionutil provides utilities for querying the system's Ash and Lacros versions.
package versionutil

import (
	"context"
	"encoding/json"
	"io/ioutil"

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
		return version.Version{}, errors.New("invalid Ash Chrome version: " + versionStr)
	}
	return v, nil
}

// RootfsLacrosVersion returns the version of Rootfs Lacros Chrome.
func RootfsLacrosVersion() (version.Version, error) {
	metadata, err := ioutil.ReadFile("/opt/google/lacros/metadata.json")
	if err != nil {
		return version.Version{}, err
	}
	metadataJSON := lacrosMetadata{}
	if err := json.Unmarshal(metadata, &metadataJSON); err != nil {
		return version.Version{}, errors.Wrap(err, "failed to parse Rootfs Lacros Chrome version")
	}
	versionStr := version.VersionRegexp.FindString(metadataJSON.Content.Version)
	v := version.Parse(versionStr)
	if !v.IsValid() {
		return version.Version{}, errors.New("invalid Rootfs Lacros Chrome version: " + versionStr)
	}
	return v, nil
}
