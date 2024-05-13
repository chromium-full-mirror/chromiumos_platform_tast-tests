// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sof

import (
	"context"
	"encoding/json"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// ProfileArtifact is the inner struct for Profile.
type ProfileArtifact struct {
	Name         string `json:"name"`
	Prefix       string `json:"prefix"`
	IsSymlink    bool   `json:"is_symlink"`
	ResolvedPath string `json:"resolved_path"`
}

// Profile is the unmarshalled struct from sof_helper devtool dump.
type Profile struct {
	Firmware ProfileArtifact `json:"fw"`
	Topology ProfileArtifact `json:"tplg"`
}

var errProfileNoInquiry = errors.New("no inquiry into profile")

// IsProfileNoInquiry tells if the given error is of no inquiry.
func IsProfileNoInquiry(err error) bool {
	return errors.Is(err, errProfileNoInquiry)
}

// GetProfile fetches data in Profile from sof_helper devtool dump.
func GetProfile(ctx context.Context) (*Profile, error) {
	cmd := testexec.CommandContext(ctx, "sof_helper", "profile", "--json")
	stdout, stderr, err := cmd.SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		// There is no inquiry into SOF profile.
		if strings.Contains(string(stderr), "cannot find profile") {
			return nil, errProfileNoInquiry
		}
		return nil, errors.Wrap(err, "call sof_helper")
	}

	prof := &Profile{}
	if err := json.Unmarshal(stdout, prof); err != nil {
		return nil, errors.Wrap(err, "unmarshal SOF profile")
	}
	return prof, nil
}
