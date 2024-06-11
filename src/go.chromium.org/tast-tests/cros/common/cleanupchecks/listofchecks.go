// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cleanupchecks

import (
	"bytes"
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
	"CheckIfDeviceIsEnrolled":             {Check: checkIfDeviceIsEnrolled, RecordState: recordStateDeviceEnrolled, Level: Fast, Enabled: true},
	"checkIfDNSChanged":                   {Check: checkIfDNSChanged, RecordState: recordDNSStatus, Level: Fast, Enabled: true},
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

// recordStateDeviceEnrolled records the enrolled state of the device before running the test.
func recordStateDeviceEnrolled(ctx context.Context) (*cs.PreTestData, error) {
	cmdline, err := exec.Command("device_management_client", "--action=install_attributes_get", "--name=enterprise.owned").Output()
	if err != nil {
		return nil, errors.New("recordStateDeviceEnrolled: failed to read kernel cmdline")
	}

	// Check if the attribute exists and is set to "true".
	isEnrolled := strings.Contains(string(cmdline), "true")
	data := &cs.PreTestData{
		Data: &cs.PreTestData_BoolData{
			BoolData: isEnrolled,
		},
	}
	return data, nil
}

// checkIfDeviceIsEnrolled checks the device enrolled state after the test and compare it with the state before the test.
func checkIfDeviceIsEnrolled(ctx context.Context, data *cs.PreTestData) error {
	cmdline, err := exec.Command("device_management_client", "--action=install_attributes_get", "--name=enterprise.owned").Output()
	if err != nil {
		return errors.New("checkIfDeviceIsEnrolled: failed to read kernel cmdline")
	}

	// Check if the attribute exists and is set to "true".
	isEnrolled := strings.Contains(string(cmdline), "true")
	if data.GetBoolData() != isEnrolled {
		return errors.Errorf("checkIfDeviceIsEnrolled: device enrolled state has changed; before: %v after: %v", data.GetBoolData(), isEnrolled)
	}
	return nil
}

// recordDNSStatus records the DNS settings before running the test.
func recordDNSStatus(ctx context.Context) (*cs.PreTestData, error) {
	currentDNSServers, err := extractDNSServers()
	if err != nil {
		return nil, errors.New("recordDNSStatus: failed to get DNS status")
	}

	data := &cs.PreTestData{
		Data: &cs.PreTestData_ByteData{
			ByteData: currentDNSServers,
		},
	}
	return data, nil
}

// checkIfDNSChanged checks the device DNS settings after the test and compare it with the settings before the test.
func checkIfDNSChanged(ctx context.Context, data *cs.PreTestData) error {
	currentDNSServers, err := extractDNSServers()
	if err != nil {
		return errors.New("checkIfDNSChanged: failed to get DNS status")
	}

	if !bytes.Equal(data.GetByteData(), currentDNSServers) {
		return errors.Errorf("checkIfDNSChanged: DNS servers have changed; before: %s after: %s", data.GetByteData(), currentDNSServers)
	}
	return nil
}

// extractDNSServers extracts the DNS servers from the output of the resolvectl status command.
func extractDNSServers() ([]byte, error) {
	cmdline, err := exec.Command("cat", "/etc/resolv.conf").Output()
	if err != nil {
		return nil, errors.New("extractDNSServers: failed to read kernel cmdline")
	}

	var dnsServers []string
	for _, line := range strings.Split(string(cmdline), "\n") {
		if strings.HasPrefix(line, "nameserver") {
			dnsServers = append(dnsServers, strings.TrimSpace(strings.TrimPrefix(line, "nameserver")))
		}
	}
	var buf bytes.Buffer
	for _, str := range dnsServers {
		buf.WriteString(str)
	}
	return buf.Bytes(), nil
}
