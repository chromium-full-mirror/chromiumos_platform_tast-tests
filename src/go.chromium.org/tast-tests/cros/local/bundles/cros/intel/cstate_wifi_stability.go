// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var intelCStateWifiIterationsVar = testing.RegisterVarString(
	"intel.CStateWifi.iterations",
	"10",
	"The number of iterations to run the C-state and WiFi stability check",
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CStateWifiStability,
		Desc:         "Check C-state values and WiFi stability",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com", "sangram.k.y@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:intel-nda"},
		Fixture:      "chromeLoggedIn",
		Timeout:      30 * time.Minute,
	})
}

// CStateWifiStability checks the C-state values and WiFi stability.
func CStateWifiStability(ctx context.Context, s *testing.State) {
	// Set up a short context for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	varValue := intelCStateWifiIterationsVar.Value()
	iterations, err := strconv.Atoi(varValue)
	if err != nil {
		s.Fatal("Failed to parse cycle count: ", err)
	}

	if err := testexec.CommandContext(ctx, "set_power_policy", "--battery_idle_action=do_nothing", "--ac_idle_action=do_nothing", "--battery_idle_delay=5", "--ac_idle_delay=5").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to change power policy: ", err)
	}
	defer func() {
		if err := testexec.CommandContext(cleanupCtx, "set_power_policy", "reset").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to reset power policy: ", err)
		}
	}()

	const sysfsFile = "/sys/kernel/debug/pmc_core/package_cstate_show"
	cpuLoads := []int{80, 70, 60, 50, 40, 30, 20, 10}
	majorPCStates := []string{"Package C2", "Package C6", "Package C8", "Package C10"}
	failureCount := 0

	for i := 0; i < iterations; i++ {
		s.Log("Starting cycle: ", i+1)

		first, err := readCStateValues(sysfsFile)
		if err != nil {
			s.Error("Failed to read first C-state values: ", err)
			continue
		}

		s.Log("First sample values")
		printCStateValues(ctx, first)

		for _, load := range cpuLoads {
			s.Logf("Running CPU load at %d%%", load)
			testexec.CommandContext(ctx, "stress-ng",
				"--cpu", "1",
				"--cpu-method", "idct",
				"-t", "20s",
				"--metrics-brief",
				"--cpu-load", strconv.Itoa(load),
				"--temp-path", "/tmp").Run(testexec.DumpLogOnError)
		}

		second, err := readCStateValues(sysfsFile)
		if err != nil {
			s.Error("Failed to read second C-state values: ", err)
			continue
		}

		s.Log("Second sample values:")
		printCStateValues(ctx, second)

		differences, err := calculateDifferences(first, second)
		if err != nil {
			s.Error("Failed to calculate differences: ", err)
			continue
		}

		s.Log("Differences between samples:")
		// Sort the differences map by state name before logging.
		keys := make([]string, 0, len(differences))
		for state := range differences {
			keys = append(keys, state)
		}
		sort.Strings(keys)

		for _, state := range keys {
			diff := differences[state]
			s.Logf("%s - %d", state, diff)
		}

		residency := calculateResidency(differences)

		s.Log("Residency percentages:")
		for _, state := range keys {
			s.Logf("%s - %.2f%%", state, residency[state]*100)
		}

		hasError := false
		for _, state := range keys {
			for _, mState := range majorPCStates {
				if mState == state && residency[state] < 0.1 {
					s.Logf("Failed as residency value %v is less than 10%% for %v", residency[state], state)
					hasError = true
				}
			}
		}

		if !hasError {
			s.Log("Success: All major PC states have residency more than 10%")
		} else {
			s.Log("Failure: Some major PC states have residency less than 10%")
			failureCount++
		}

		if err := checkWiFiStability(ctx); err != nil {
			s.Fatal("Failed to check WiFi stability: ", err)
		}
	}

	if failureCount > 0 {
		s.Fatalf("Number of cycles where major PC states did not reach 10%% residency: %d", failureCount)
	}
}

func readCStateValues(filePath string) (map[string]int64, error) {
	cstateValues := make(map[string]int64)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Read the file line by line.
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			return nil, errors.Errorf("failed to split the line into 2 parts: %s", line)
		}
		state := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])
		value, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse value %s", valueStr)
		}
		cstateValues[state] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "failed to scan the file")
	}
	return cstateValues, nil
}

func printCStateValues(ctx context.Context, cstateValues map[string]int64) {
	// Sort the differences map by state name before logging.
	keys := make([]string, 0, len(cstateValues))
	for state := range cstateValues {
		keys = append(keys, state)
	}
	sort.Strings(keys)

	testing.ContextLog(ctx, "C-state values:")
	for _, state := range keys {
		testing.ContextLogf(ctx, "%v : %v", state, cstateValues[state])
	}
}

func calculateDifferences(oldValues, newValues map[string]int64) (map[string]int64, error) {
	diffs := make(map[string]int64)
	for state, oldValue := range oldValues {
		newValue, ok := newValues[state]
		if !ok {
			return nil, errors.Errorf("state %s not found in new values", state)
		}
		diffs[state] = newValue - oldValue
	}
	return diffs, nil
}

// calculateResidency calculates the residency ratio for each state in value [0.0 - 1.0] (inclusive).
// The residency is degenerated if the totalDiff is 0.
func calculateResidency(diffs map[string]int64) map[string]float64 {
	res := make(map[string]float64)
	totalDiff := int64(0)
	for _, diff := range diffs {
		totalDiff += diff
	}

	if totalDiff == 0 {
		for state := range diffs {
			res[state] = 0
		}
	} else {
		for state, diff := range diffs {
			res[state] = float64(diff) / float64(totalDiff)
		}
	}
	return res
}

func checkWiFiStability(ctx context.Context) error {
	// Run the lspci command to get the list of PCI devices.
	output, err := testexec.CommandContext(ctx, "lspci").Output()
	if err != nil {
		return errors.Wrap(err, "failed to enumerate PCI devices")
	}

	// Iterate over the output lines to find the network controller.
	devEnumeration, err := findMatchingLine(output, "Network controller: Intel Corporation")
	if err != nil {
		return errors.Wrap(err, "failed to find network controller")
	}
	if devEnumeration == "" {
		return errors.New("wifi: device enumeration failed")
	}
	testing.ContextLogf(ctx, "wifi: device enumeration successful. dev_enumeration: %s", devEnumeration)

	// Check if the wifi driver is loaded.
	driverLoading, err := testexec.CommandContext(ctx, "lsmod").Output()
	if err != nil {
		return errors.Wrap(err, "failed to check loaded modules")
	}
	driverLine, err := findMatchingLine(driverLoading, "iwlwifi")
	if err != nil {
		return errors.Wrap(err, "failed to find wifi driver")
	}
	if driverLine == "" {
		return errors.New("wifi: driver loading failed")
	}
	testing.ContextLogf(ctx, "wifi: driver loading successful. wifi_driver: %s", driverLine)

	// Get the BDF of the network controller.
	bdf := strings.Fields(devEnumeration)[0]
	firmwarePath := fmt.Sprintf("/sys/kernel/debug/iwlwifi/0000:%s/iwlmvm/fw_ver", bdf)
	firmwareLoad, err := os.ReadFile(firmwarePath)
	if err != nil || !strings.Contains(string(firmwareLoad), "core") {
		return errors.New("wifi: firmware loading failed")
	}
	testing.ContextLogf(ctx, "wifi: firmware loading successful. firmware_load: %s", string(firmwareLoad))

	// Check for any assert errors in dmesg.
	assertErr, err := testexec.CommandContext(ctx, "dmesg").Output()
	if err == nil && strings.Contains(string(assertErr), "ADVANCED_SYSASSERT") {
		return errors.New("wifi: assert error observed")
	}

	// Clear dmesg logs.
	if err := testexec.CommandContext(ctx, "dmesg", "-c").Run(); err != nil {
		return errors.Wrap(err, "failed to clear dmesg logs")
	}
	return nil
}

func findMatchingLine(output []byte, match string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, match) {
			return line, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", errors.Wrap(err, "failed to scan the output")
	}
	return "", nil
}
