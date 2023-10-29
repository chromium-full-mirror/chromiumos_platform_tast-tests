// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"bytes"
	"context"
	"os"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FlexHWIS,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the ChromeOS Flex HWIS can run and exit successfully",
		Contacts: []string{
			"chromeos-flex-eng@google.com",
			"tinghaolin@google.com", // Test author
		},
		BugComponent: "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		SoftwareDeps: []string{"chrome", "flex_internal"},
		Attr:         []string{"group:criticalstaging", "group:mainline", "informational"},
		Fixture:      fixture.ChromeEnrolledLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceSystemInfo{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceCpuInfo{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceGraphicsStatus{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceMemoryInfo{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceVersionInfo{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceNetworkConfiguration{}, pci.Served),
		},
	})
}

const (
	hwisTimeFileName    = "/var/lib/flex_hwis_tool/time"
	hwisSuccessResponse = "flex_hwis_tool ran successfully"
)

func FlexHWIS(ctx context.Context, s *testing.State) {
	// For all environment settings, refer to the test of the device
	// policy report, such as /cros/policy/display_reporting_dbus.go
	s.Log("Set up the environment to enable the relevant policies")
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	policies := []policy.Policy{
		// Set these policies to true to allow registered devices to send
		// hardware information.
		&policy.ReportDeviceSystemInfo{Stat: policy.StatusSet, Val: true},
		&policy.ReportDeviceCpuInfo{Stat: policy.StatusSet, Val: true},
		&policy.ReportDeviceGraphicsStatus{Stat: policy.StatusSet, Val: true},
		&policy.ReportDeviceMemoryInfo{Stat: policy.StatusSet, Val: true},
		&policy.ReportDeviceVersionInfo{Stat: policy.StatusSet, Val: true},
		&policy.ReportDeviceNetworkConfiguration{Stat: policy.StatusSet, Val: true},
	}

	// Update policies.
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	s.Log("Run and test the HWIS service")
	// The flex_hwis_tool service will check the /var/lib/flex_hwis_tool/time
	// file before running and make sure that it has not been run within the
	// specified time. To ensure that the service can run, remove possible
	// time file before running the command.
	os.Remove(hwisTimeFileName)
	out, err := testexec.CommandContext(ctx, "flex_hwis_tool", "--debug").CombinedOutput()
	// The flex_hwis_tool service will create a file to record the time after
	// successfully running. The service will not run again within the specified
	// time period. To ensure the test can run at any time, the time file must be
	// removed after the test is complete.
	os.Remove(hwisTimeFileName)

	outString := string(out)
	if err != nil {
		s.Fatalf("flex_hwis_tool fails to run and outputs: %s", outString)
	}
	if !bytes.Contains(out, []byte(hwisSuccessResponse)) {
		s.Fatalf("flex_hwis_tool doesn't contain success string and output: %s", outString)
	}
}
