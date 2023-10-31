// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// List of cros_healthd diagnostic routines
const (
	RoutineBatteryCapacity         string = "battery_capacity"
	RoutineBatteryHealth                  = "battery_health"
	RoutineURandom                        = "urandom"
	RoutineSmartctlCheck                  = "smartctl_check"
	RoutineACPower                        = "ac_power"
	RoutineCPUCache                       = "cpu_cache"
	RoutineCPUStress                      = "cpu_stress"
	RoutineFloatingPointAccurary          = "floating_point_accuracy"
	RoutineNVMEWearLevel                  = "nvme_wear_level"
	RoutineNVMESelfTest                   = "nvme_self_test"
	RoutineDiskRead                       = "disk_read"
	RoutinePrimeSearch                    = "prime_search"
	RoutineBatteryDischarge               = "battery_discharge"
	RoutineBatteryCharge                  = "battery_charge"
	RoutineMemory                         = "memory"
	RoutineLanConnectivity                = "lan_connectivity"
	RoutineSignalStrength                 = "signal_strength"
	RoutineGatewayCanBePinged             = "gateway_can_be_pinged"
	RoutineHasSecureWifiConnection        = "has_secure_wifi_connection"
	RoutineDNSResolverPresent             = "dns_resolver_present"
	RoutineDNSLatency                     = "dns_latency"
	RoutineDNSResolution                  = "dns_resolution"
	RoutineCaptivePortal                  = "captive_portal"
	RoutineHTTPFirewall                   = "http_firewall"
	RoutineHTTPSFirewall                  = "https_firewall"
	RoutineHTTPSLatency                   = "https_latency"
	RoutineSensitiveSensor                = "sensitive_sensor"
	RoutineFingerprint                    = "fingerprint"
	RoutineFingerprintAlive               = "fingerprint_alive"
	RoutineEMMCLifetime                   = "emmc_lifetime"
	RoutineBluetoothPower                 = "bluetooth_power"
	RoutineBluetoothDiscovery             = "bluetooth_discovery"
	RoutineBluetoothScanning              = "bluetooth_scanning"
	RoutinePowerButton                    = "power_button"
)

// List of possible routine statuses
const (
	StatusReady         string = "Ready"
	StatusRunning              = "Running"
	StatusWaiting              = "Waiting"
	StatusPassed               = "Passed"
	StatusFailed               = "Failed"
	StatusError                = "Error"
	StatusCancelled            = "Cancelled"
	StatusFailedToStart        = "Failed to start"
	StatusRemoved              = "Removed"
	StatusCancelling           = "Cancelling"
	StatusUnsupported          = "Unsupported"
	StatusNotRun               = "Not run"
)

// RoutineResult contains the progress of the routine as a percentage and
// the routine status.
type RoutineResult struct {
	Progress      int
	Status        string
	StatusMessage string
}

// VerifyPassed returns nil if the routine status is passed and the progress is 100. Returns an error otherwise.
func (result RoutineResult) VerifyPassed() error {
	if result.Status != StatusPassed {
		return errors.Errorf("unexpected status: got %q, want %q; message = %q", result.Status, StatusPassed, result.StatusMessage)
	}
	if result.Progress != 100 {
		return errors.Errorf("unexpected progress: got %d, want 100; message = %q", result.Progress, result.StatusMessage)
	}
	return nil
}

// VerifyFinished returns nil if both the following conditions hold
// (1) the routine status is either passed, failed or not run,
// (2) if the status is passed or failed, the routine progress is 100.
// Returns an error otherwise.
//
// It verifies the routine completes successfully without crashing or throwing
// errors. For example, some lab machines might have old batteries that would
// fail the battery diagnostic routines, but this is regarded as a "successful
// run".
func (result RoutineResult) VerifyFinished() error {
	if result.Status != StatusPassed && result.Status != StatusFailed && result.Status != StatusNotRun {
		return errors.Errorf("unexpected status: got %q, want %q, %q, or %q; message = %q",
			result.Status, StatusPassed, StatusFailed, StatusNotRun, result.StatusMessage)
	}
	if result.Progress != 100 && result.Status != StatusNotRun {
		return errors.Errorf("unexpected progress: got %d, want 100; status = %q; message = %q",
			result.Progress, result.Status, result.StatusMessage)
	}
	return nil
}

// RoutineParams are different configuration options for running a diagnostic
// routine.
type RoutineParams struct {
	Routine                       string // The name of the routine to run
	Cancel                        bool   // Boolean flag to cancel the routine
	DefaultNVMEWearLevelThreshold int    // Threshold for RoutineNVMEWearLevel. The param
	// will only be used if the corresponding field in
	// cros-config is missing.
}

// NewRoutineParams creates and returns a diagnostic routine with default test
// parameters.
func NewRoutineParams(routine string) RoutineParams {
	return RoutineParams{
		Routine:                       routine,
		Cancel:                        false,
		DefaultNVMEWearLevelThreshold: 50,
	}
}

// RunDiagRoutine runs the specified routine based on `params`. Returns a
// RoutineResult on success or an error.
func RunDiagRoutine(ctx context.Context, params RoutineParams) (*RoutineResult, error) {
	testing.ContextLogf(ctx, "Running routine: %s", params.Routine)
	diagParams := []string{params.Routine}
	if params.Cancel {
		diagParams = append(diagParams, "--force_cancel_at_percent=5")
	}
	if params.Routine == RoutineNVMEWearLevel {
		threshold, err := getNVMEWearLevelThreshold(ctx, params.DefaultNVMEWearLevelThreshold)
		if err != nil {
			return nil, errors.Wrap(err, "failed to prepare NVME wear-level-threshold")
		}
		diagParams = append(diagParams, fmt.Sprintf("--wear_level_threshold=%d", threshold))
	} else if params.Routine == RoutineBluetoothScanning {
		// Default runtime for Bluetooth scanning routine is 5 seconds.
		diagParams = append(diagParams, "--length_seconds=5")
	} else if params.Routine == RoutineMemory {
		// 15000 KiB runs for about 3 seconds on a volteer machine
		diagParams = append(diagParams, "--max_testing_mem_kib=15000")
	} else if params.Routine == RoutineCPUCache || params.Routine == RoutineCPUStress ||
		params.Routine == RoutineFloatingPointAccurary || params.Routine == RoutinePrimeSearch ||
		params.Routine == RoutineURandom {
		// Run routine for 1 second to verify it doesn't crash. These routines
		// share the same command line argument `--length_seconds`.
		diagParams = append(diagParams, "--length_seconds=1")
	} else if params.Routine == RoutineDiskRead {
		diagParams = append(diagParams, "--length_seconds=1")
		diagParams = append(diagParams, "--file_size_mb=64")
	} else if params.Routine == RoutinePowerButton {
		diagParams = append(diagParams, "--length_seconds=5")
	}

	var output string
	var err error
	if params.Routine == RoutinePowerButton {
		output, err = runPowerButtonDiag(ctx, diagParams)
	} else {
		output, err = runDiag(ctx, diagParams)
	}
	if err != nil {
		return nil, err
	}
	return parseDiagOutput(ctx, output)
}

// GetDiagRoutines returns a list of valid routines for the device on success,
// or an error.
func GetDiagRoutines(ctx context.Context) ([]string, error) {
	output, err := runDiag(ctx, []string{"get_routines"})
	if err != nil {
		return []string{}, err
	}

	re := regexp.MustCompile(`Available routine: (.*)`)
	var routines []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		match := re.FindStringSubmatch(line)
		if match != nil {
			routines = append(routines, match[1])
		}
	}
	return routines, nil
}

// runDiag is a helper function that runs the cros_healthd diag command and
// returns the raw stdout on success, or an error.
func runDiag(ctx context.Context, args []string) (string, error) {
	args = append([]string{"diag"}, args...)
	cmd := testexec.CommandContext(ctx, "cros-health-tool", args...)
	testing.ContextLogf(ctx, "Running %q", shutil.EscapeSlice(cmd.Args))
	stdout, stderr, err := cmd.SeparatedOutput()
	if err != nil {
		cmd.DumpLog(ctx)
		return "", errors.Wrapf(err, "command failed with stdout: %q, stderr: %q", string(stdout), string(stderr))
	}
	return string(stdout), nil
}

// runPowerButtonDiag is a helper function similar to `runDiag` while simulating the
// power button event for power button routine.
func runPowerButtonDiag(ctx context.Context, args []string) (string, error) {
	// Helper function to send events.
	sendPowerButtonEvent := func(ew *input.RawEventWriter, val int32) error {
		if err := ew.Event(input.EV_KEY, input.KEY_POWER, val); err != nil {
			return errors.Wrapf(err, "failed to emit power button event with val = %d", val)
		}
		if err := ew.Sync(); err != nil {
			return errors.Wrapf(err, "failed to sync power button event with val = %d", val)
		}
		return nil
	}
	pressPowerButton := func(ew *input.RawEventWriter) error {
		return sendPowerButtonEvent(ew, 1)
	}
	releasePowerButton := func(ew *input.RawEventWriter) error {
		return sendPowerButtonEvent(ew, 0)
	}

	// Find power button device.
	deviceFound, powerButtonDevicePath, err := input.FindPowerKeyDevice(ctx)
	if err != nil {
		return "", err
	} else if !deviceFound {
		return "", errors.New("no input device for power button found")
	}
	testing.ContextLogf(ctx, "power button device found: %q", powerButtonDevicePath)

	// Open the input device.
	powerButtonEventWriter, err := input.Device(ctx, powerButtonDevicePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to open power button device")
	}
	defer releasePowerButton(powerButtonEventWriter)
	defer powerButtonEventWriter.Close()

	obj, err := dbusutil.NewDBusObject(ctx, "org.chromium.PowerManager", "org.chromium.PowerManager", "/org/chromium/PowerManager")
	if err != nil {
		return "", errors.Wrap(err, "failed to connect to PowerManager D-Bus service")
	}

	// Start cros_healthd routine.
	var stdoutBuf bytes.Buffer
	args = append([]string{"diag"}, args...)
	runRoutineCmd := testexec.CommandContext(ctx, "cros-health-tool", args...)
	runRoutineCmd.Stdout = &stdoutBuf
	if err := runRoutineCmd.Start(); err != nil {
		testing.ContextLogf(ctx, "stdout of command: %q", stdoutBuf.String())
		runRoutineCmd.DumpLog(ctx)
		return "", errors.Wrapf(err, "failed to run %q", shutil.EscapeSlice(runRoutineCmd.Args))
	}
	testing.ContextLogf(ctx, "Running %q", shutil.EscapeSlice(runRoutineCmd.Args))

	// Toggle power button pressed and released state repeatedly until the routine finishes.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Ignore power button press for 1_000_000 microseconds = 1 seconds.
		// This is to avoid shutting down the DUT by accident.
		if err := obj.Call(ctx, "IgnoreNextPowerButtonPress", int64(1_000_000)).Err; err != nil {
			return errors.Wrap(err, "failed to call IgnoreNextPowerButtonPress")
		}
		if err := pressPowerButton(powerButtonEventWriter); err != nil {
			return err
		}
		if err := releasePowerButton(powerButtonEventWriter); err != nil {
			return err
		}
		// Setting the timeout to 0 to cancel the previously set period.
		if err := obj.Call(ctx, "IgnoreNextPowerButtonPress", int64(0)).Err; err != nil {
			return errors.Wrap(err, "failed to cancel IgnoreNextPowerButtonPress")
		}
		if strings.Contains(stdoutBuf.String(), "Status message") {
			return nil
		}
		return errors.New("routine not finished")
	}, &testing.PollOptions{Interval: 1 * time.Second,
		// The routine runs for 5 seconds. Add extra 2 seconds as a buffer.
		Timeout: 7 * time.Second}); err != nil {
		return "", errors.Wrap(err, "routine timeout")
	}

	if err := runRoutineCmd.Wait(); err != nil {
		return "", errors.Wrap(err, "failed to wait command")
	}
	return stdoutBuf.String(), nil
}

// getNVMEWearLevelThreshold reads the threshold value for NVME wear level from
// cros-config. Fallback to `defaultValue` if the corresponding property is not
// defined in cros-config.
func getNVMEWearLevelThreshold(ctx context.Context, defaultValue int) (int, error) {
	thresholdStr, err := crosconfig.Get(ctx, "/cros-healthd/routines/nvme-wear-level", "wear-level-threshold")
	if err != nil {
		if crosconfig.IsNotFound(err) {
			return defaultValue, nil
		}
		return 0, errors.Wrap(err, "failed to invoke cros_config for wear-level-threshold")
	}

	threshold, err := strconv.Atoi(thresholdStr)
	if err != nil {
		return 0, errors.Wrapf(err, "Unable to parse wear-level-threshold in cros_config %q to int", thresholdStr)
	}
	return threshold, nil
}

// parseDiagOutput is a helper function that takes the `raw` output from running a
// diagnostic routine and returns a RoutineResult on success, or an error.
//
// Some examples for `raw`:
// "\rProgress: 0\rProgress: 100\rProgress: 100\nStatus: Passed\nStatus message: Routine passed.\n"
// "\rProgress: 25\nInteractive message.\n\rProgress: 100\rProgress: 100\nStatus: Passed\nStatus message: Routine passed.\n"
func parseDiagOutput(ctx context.Context, raw string) (*RoutineResult, error) {
	status := ""
	statusMessage := ""
	progress := 0
	re := regexp.MustCompile(`([^:]+): (.*)`)
	testing.ContextLog(ctx, raw)

	// Treat both \n and \r as separators since the purpose of \r is to make the
	// output more readable in a terminal.
	for _, line := range regexp.MustCompile(`(\n|\r)`).Split(raw, -1) {
		match := re.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		key := match[1]
		value := match[2]
		switch key {
		case "Status":
			status = value
		case "Progress":
			i, err := strconv.Atoi(value)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to parse Progress value %q as int", value)
			}
			// Override the old value because only the last progress will be reported.
			progress = i
		case "Status message":
			statusMessage = value
		}
	}
	return &RoutineResult{progress, status, statusMessage}, nil
}
