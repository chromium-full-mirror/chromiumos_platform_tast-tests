// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"fmt"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PmTestMode is mode listed in https://www.kernel.org/doc/Documentation/power/basic-pm-debugging.txt
type PmTestMode string

// All available pm_test mode listed in https://www.kernel.org/doc/Documentation/power/basic-pm-debugging.txt
// The list is sorted from less invasive mode to most invasive mode"
const (
	PmTestNone       PmTestMode = "none"       // PmTestNone is the mode by default
	PmTestFreezer               = "freezer"    // PmTestFreezer tests the freezing of processes
	PmTestDevices               = "devices"    // PmTestDevices tests PmTestFreezer + suspending of devices
	PmTestPlatform              = "platform"   // PmTestPlatform tests PmTestDevices + platform global control methods(*)
	PmTestProcessors            = "processors" // PmTestProcessors tests PmTestPlatform + the disabling of nonboot CPUs
	PmTestCore                  = "core"       // PmTestCore tests PmTestProcessors + suspending of platform/system devices
)

type pmTestStatus struct {
	Current   PmTestMode   // Current pm_test state
	Available []PmTestMode // Supported pm_test state
}

const (
	pmTestPath = "/sys/power/pm_test"
)

// GetPMTestState returns the current PmTestMode as well as all supported PmTestMode reported by /sys/power/pm_test.
func GetPMTestState(ctx context.Context) (pmTestStatus, error) {
	state := pmTestStatus{}

	CurrentRegexp := regexp.MustCompile(`\[(.+)\]`)
	availableRegexp := regexp.MustCompile(`\b(\S+)\b`)
	out, err := testexec.CommandContext(ctx, "cat", pmTestPath).Output(testexec.DumpLogOnError)
	if err != nil {
		return state, errors.Wrapf(err, "failed to read %v", pmTestPath)
	}
	outStr := string(out)

	match := CurrentRegexp.FindStringSubmatch(outStr)
	if match == nil {
		return state, errors.Errorf("failed to list the current pm_test state: %q", outStr)
	}
	state.Current = PmTestMode(match[1])

	matches := availableRegexp.FindAllStringSubmatch(outStr, -1)
	if matches == nil {
		return state, errors.Errorf("failed to list all available pm_test state: %q", outStr)
	}
	for _, m := range matches {
		state.Available = append(state.Available, PmTestMode(string(m[1])))
	}
	return state, nil
}

// SetPMTest sets the pm_test to corresponding mode.
func SetPMTest(ctx context.Context, mode PmTestMode) error {
	testing.ContextLog(ctx, "Set pm_test mode to ", mode)
	cmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo %v > %v", mode, pmTestPath))
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to write %v to %v", mode, pmTestPath)
	}
	state, err := GetPMTestState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get pm_test state")
	}
	if state.Current != mode {
		return errors.Errorf("failed to verify pm_test changed to %v, got: %v", mode, state.Current)
	}
	return nil
}

// SuspendMode controls whether power_manager should suspend to S0ix or S3
type SuspendMode int

// Supported modes cited in https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/power_manager/docs/suspend_stress_test.md.
const (
	SuspendS3   SuspendMode = 0
	SuspendS0ix             = 1
)

const (
	suspend2IdlePath = "/var/lib/power_manager/suspend_to_idle"
)

func (m SuspendMode) String() string {
	if m == SuspendS3 {
		return "S3"
	} else if m == SuspendS0ix {
		return "S0ix"
	}
	return ""
}

// GetSuspendState returns the current configuration of suspend_to_idle by the power_manager.
func GetSuspendState(ctx context.Context) SuspendMode {
	err := testexec.CommandContext(ctx, "check_powerd_config", "--suspend_to_idle").Run(testexec.DumpLogOnError)
	if err == nil {
		return SuspendS0ix
	}
	return SuspendS3
}
