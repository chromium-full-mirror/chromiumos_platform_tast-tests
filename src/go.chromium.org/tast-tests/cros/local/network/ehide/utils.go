// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ehide

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"go.chromium.org/tast-tests/cros/common/network/ehideconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// IsOn checks whether ehide is on.
func IsOn(ctx context.Context) (bool, error) {
	state, err := getState(ctx, ehideconst.StateFilePath)
	if err != nil {
		return false, err
	}
	return state == ehideconst.EhideStateOn, nil
}

// getState returns the ehide state by reading path, or falls back to running
// "ehide state".
func getState(ctx context.Context, path string) (string, error) {
	// "ehide state" only reads the state file, but running the Python script
	// takes ~150 ms on low-end DUTs, and test hooks check the state before
	// every test. So read the file directly.
	b, err := os.ReadFile(path)
	if err == nil { // if NO error
		return strings.TrimSpace(string(b)), nil
	}
	// ehide creates the file when it starts for the first time in a boot, so
	// a missing file means off as long as ehide is installed.
	if errors.Is(err, os.ErrNotExist) {
		if _, err := exec.LookPath(ehideconst.EhidePath); err == nil { // if NO error
			return ehideconst.EhideStateOff, nil
		}
	}

	// Let ehide handle the other cases, e.g. report an error if not installed.
	output, err := testexec.CommandContext(ctx, ehideconst.EhidePath, "state").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(string(output))
	return state, nil
}
