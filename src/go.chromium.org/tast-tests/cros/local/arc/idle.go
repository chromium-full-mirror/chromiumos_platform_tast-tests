// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DeviceIdleState is the mState string dumped by `dumpsys deviceidle`.
// Here just list the states which will be used on ARC. For all states, please
// refer Android AOSP DeviceIdleController.java code.
type DeviceIdleState string

const (
	// DeviceActive is active state string.
	DeviceActive DeviceIdleState = "ACTIVE"

	// DeviceInActive is the inactive state string.
	DeviceInActive DeviceIdleState = "INACTIVE"

	// DeviceIdlePending is the idle pending state string.
	DeviceIdlePending DeviceIdleState = "IDLE_PENDING"

	// DeviceIdle is the idle state string.
	DeviceIdle DeviceIdleState = "IDLE"
)

// IsValid determines if the string is a valid value for device idle state.
func (state DeviceIdleState) IsValid() error {
	switch state {
	case DeviceActive, DeviceInActive, DeviceIdlePending, DeviceIdle:
		return nil
	}
	return errors.New("invalid DeviceIdleState")
}

// IsDoze returns if the |state| of ARC is a doze state.
func (state DeviceIdleState) IsDoze() bool {
	return state != DeviceActive
}

var (
	// Looking for:
	//   mState=... mLightState=...
	dozeStateRE = regexp.MustCompile(`(?m)` + // Enable multiline.
		`mState=([\W\w]+?)\s`) // Gather state string.
)

// DeviceIdleState returns device idle state.
func (a *ARC) DeviceIdleState(ctx context.Context) (DeviceIdleState, error) {
	cmd := a.Command(ctx, "dumpsys", "deviceidle")
	output, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to execute 'dumpsys deviceidle'")
	}

	group := dozeStateRE.FindStringSubmatch(string(output))
	if len(group) != 2 {
		outputDir, ok := testing.ContextOutDir(ctx)
		if !ok {
			return "", errors.New("failed to get the testing output directory")
		}
		if err := os.WriteFile(filepath.Join(outputDir, "dumpsys_deviceidle.txt"), output, 0644); err != nil {
			return "", errors.Wrap(err, "failed to write invalid dumpsys output to file")
		}
		return "", errors.New("failed to find device idle info from `dumpsys deviceidle`, the dumpsys result has been written to dumpsys_deviceidle.txt")
	}
	state := DeviceIdleState(group[1])
	if err := state.IsValid(); err != nil {
		return "", err
	}
	return state, nil
}
