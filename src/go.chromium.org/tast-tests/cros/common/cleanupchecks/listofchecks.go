// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cleanupchecks

import (
	"context"
	"os"
	"os/exec"
	"strings"

	cs "go.chromium.org/tast-tests/cros/services/cros/cleanupchecks"
	"go.chromium.org/tast/core/errors"
)

// This map contains pointers to all functions that perform a clean up check.
// Every new function to be used should be added to this map.
// The Enabled parameter should be set to true if if should be enabled by default.
// Checks that should only run in certain cases, e.g. a specific Fixture is used,
// then the Enabled parameter should be set to false here and the Fixture that
// uses it has to enable it in the SetUp function.
var listOfAllChecks = map[string]*CleanUpCheck{
	"CheckIfTastUseFlagsFileExists":       {Check: checkIfTastUseFlagsFileExists, RecordState: nil, Level: Fast, Enabled: true},
	"CheckIfTastBundlesLocalFileExists":   {Check: checkIfTastBundlesLocalFileExists, RecordState: nil, Level: Fast, Enabled: true},
	"CheckIfRootfsVerificationIsTurnedOn": {Check: checkIfRootfsVerificationIsTurnedOn, RecordState: nil, Level: Fast, Enabled: true},
}

// CleanUpChecks can be defined here or in some other package which will be imported in this file.

// CheckIfTastUseFlagsFileExists.

func checkIfTastUseFlagsFileExists(ctx context.Context, data *cs.PreTestData) error {
	_, err := os.Stat("/usr/local/etc/tast_use_flags.txt")
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("checkIfTastUseFlagsFileExists: /usr/local/etc/tast_use_flags.txt does not exist")
	}
	return nil
}

// CheckIfTastBundlesLocalFileExists.

func checkIfTastBundlesLocalFileExists(ctx context.Context, data *cs.PreTestData) error {
	_, err := os.Stat("/usr/local/libexec/tast/bundles/local/cros")
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("checkIfTastBundlesLocalFileExists: /usr/local/libexec/tast/bundles/local/cros does not exist")
	}
	return nil
}

// CheckIfRootfsVerificationIsTurnedOn.

// checkIfRootfsVerificationIsTurnedOn checks whether rootfs verification is turned
// on after a test.
func checkIfRootfsVerificationIsTurnedOn(ctx context.Context, data *cs.PreTestData) error {
	cmdline, err := exec.Command("cat", "/proc/mounts").Output()
	if err != nil {
		return errors.New("checkIfRootfsVerificationIsTurnedOn: failed to read kernel cmdline")
	}
	if !strings.Contains(string(cmdline), "dm_verity.dev_wait=1") {
		return errors.New("checkIfRootfsVerificationIsTurnedOn: Rootfs verificaiton has been turned off")
	}
	return nil
}
