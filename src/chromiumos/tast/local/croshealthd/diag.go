// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/crosconfig"
	"chromiumos/tast/testing"
	"go.chromium.org/tast/core/shutil"
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
	RoutineLedLitUp                       = "led_lit_up"
	RoutineAudioSetVolume                 = "audio_set_volume"
	RoutineAudioSetGain                   = "audio_set_gain"
	RoutineBluetoothPower                 = "bluetooth_power"
	RoutineBluetoothDiscovery             = "bluetooth_discovery"
	RoutineBluetoothScanning              = "bluetooth_scanning"
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

// RoutineParams are different configuration options for running a diagnostic
// routine.
type RoutineParams struct {
	Routine                       string // The name of the routine to run
	Cancel                        bool   // Boolean flag to cancel the routine
	DefaultNVMEWearLevelThreshold int    // Threshold for RoutineNVMEWearLevel. The param
	// will only be used if the corresponding field in
	// cros-config is missing.
}

// RunDiagRoutine runs the specified routine based on `params`. Returns a
// RoutineResult on success or an error.
func RunDiagRoutine(ctx context.Context, params RoutineParams) (*RoutineResult, error) {
	diagParams := []string{"--action=run_routine", fmt.Sprintf("--routine=%s", params.Routine)}
	if params.Cancel {
		diagParams = append(diagParams, "--force_cancel_at_percent=5")
	}
	if params.Routine == RoutineNVMEWearLevel {
		threshold, err := getNVMEWearLevelThreshold(ctx, params.DefaultNVMEWearLevelThreshold)
		if err != nil {
			return nil, errors.Wrap(err, "failed to prepare NVME wear-level-threshold")
		}
		diagParams = append(diagParams, fmt.Sprintf("--wear_level_threshold=%d", threshold))
	} else if params.Routine == RoutineLedLitUp {
		// Use an arbitrary supported LED and color for testing. Here, we use
		// the first supported LED and its first supported color from `getSupportedLED`.
		supportedLED, err := getSupportedLED(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get supported LEDs")
		}
		if len(supportedLED) == 0 {
			return nil, errors.Wrap(err, "no supported LEDs")
		}
		for ledName, ledColors := range supportedLED {
			if len(ledColors) == 0 {
				return nil, errors.Wrap(err, "the list of supported colors should not be empty")
			}
			diagParams = append(diagParams, fmt.Sprintf("--led_name=%s", ledName), fmt.Sprintf("--led_color=%s", ledColors[0]))
			break
		}
	} else if params.Routine == RoutineAudioSetVolume {
		// Any node id is fine. What we need to test is audio dbus works.
		diagParams = append(diagParams, "--node_id=0")
		diagParams = append(diagParams, "--volume=10")
	} else if params.Routine == RoutineAudioSetGain {
		// Any node id is fine. What we need to test is audio dbus works.
		diagParams = append(diagParams, "--node_id=0")
		diagParams = append(diagParams, "--gain=10")
	} else if params.Routine == RoutineBluetoothScanning {
		// Default runtime for Bluetooth scanning routine is 5 seconds.
		diagParams = append(diagParams, "--length_seconds=5")
	}

	var output string
	var err error
	if params.Routine == RoutineLedLitUp {
		output, err = runLEDDiag(ctx, diagParams)
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
	output, err := runDiag(ctx, []string{"--action=get_routines"})
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
	out, err := cmd.Output()
	if err != nil {
		cmd.DumpLog(ctx)
		return "", errors.Wrapf(err, "failed to run %q", shutil.EscapeSlice(cmd.Args))
	}
	return string(out), nil
}

// runLEDDiag is a helper function similar to `runDiag` while simulating the
// user input for LED routine.
//
// TODO(weiluanwang): Check the stdout before simulating user inputs.
func runLEDDiag(ctx context.Context, args []string) (string, error) {
	args = append([]string{"diag"}, args...)
	cmd := testexec.CommandContext(ctx, "cros-health-tool", args...)
	testing.ContextLogf(ctx, "Running %q", shutil.EscapeSlice(cmd.Args))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cmd.DumpLog(ctx)
		return "", errors.Wrap(err, "failed to get cmd.StdinPipe()")
	}

	go func() {
		defer stdin.Close()
		// Input `y` to proceed. The `y` indicates that the color is correct.
		io.WriteString(stdin, "y")
	}()

	out, err := cmd.Output()
	if err != nil {
		cmd.DumpLog(ctx)
		return "", errors.Wrapf(err, "failed to run %q", shutil.EscapeSlice(cmd.Args))
	}
	return string(out), nil
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

// getSupportedLED returns a map of the list of supported colors for each
// supported LED. For example, {"battery": ["red", "yellow", "green"], ...}.
func getSupportedLED(ctx context.Context) (map[string][]string, error) {
	re := regexp.MustCompile(`([^:]+): 0x([a-fA-F0-9]+)`)
	possibleLEDColor := map[string]bool{
		"red":    true,
		"green":  true,
		"blue":   true,
		"yellow": true,
		"white":  true,
		"amber":  true,
	}

	m := make(map[string][]string)
	for _, ledName := range []string{"battery", "power", "adapter", "left", "right"} {
		out, err := testexec.CommandContext(ctx, "ectool", "led", ledName, "query").Output()
		if err != nil {
			// The command will fail if this LED is not supported.
			testing.ContextLogf(ctx, "Failed to query brightness range for LED %q", ledName)
			continue
		}
		// Example output:
		// Brightness range for LED 0:
		//         red     : 0x1
		//         green   : 0x1
		//         blue    : 0x0
		//         yellow  : 0x0
		//         white   : 0x0
		//         amber   : 0x1
		for _, line := range strings.Split(string(out), "\n") {
			match := re.FindStringSubmatch(line)
			if match == nil {
				continue
			}

			colorName := strings.TrimSpace(match[1])
			if _, exists := possibleLEDColor[colorName]; !exists {
				testing.ContextLogf(ctx, "Invalid LED name: %q", colorName)
				continue
			}

			// Brightness range other than 0x0 means the color is supported.
			if match[2] != "0" {
				m[ledName] = append(m[ledName], colorName)
			}
		}
	}
	return m, nil
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
