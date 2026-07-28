// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"bytes"
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	commonupstart "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: HWIS,
		Desc: "Tests that the ChromeOS Flex HWIS can run and exit successfully",
		Contacts: []string{
			"chromeos-flex-eng+oncall@google.com",
			"tinghaolin@google.com", // Test author
			"tbrandston@google.com",
		},
		BugComponent: "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		SoftwareDeps: []string{"chrome", "flex_hwis"},
		Attr:         []string{"group:mainline"},
		Fixture:      fixture.ChromeEnrolledLoggedIn,
		// b/365541979: Skip on the reven\nuc11.
		HardwareDeps: hwdep.D(hwdep.SkipDMIProductName("NUC11TNKv5")),
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceFlexHwDataForProductImprovementEnabled{}, pci.VerifiedFunctionalityOS),
		},
	})
}

const (
	hwisTimeFileName          = "/var/lib/flex_hwis_tool/time"
	hwisSuccessResponse       = "flex_hwis_tool sent successfully"
	hwisNotAuthorizedResponse = "flex_hwis_tool wasn't authorized to send data"
)

func HWIS(ctx context.Context, s *testing.State) {
	for _, test := range []struct {
		name             string
		policies         []policy.Policy
		expectedResponse string
	}{
		{
			"default",
			[]policy.Policy{},
			hwisSuccessResponse,
		},
		{
			"disabled",
			[]policy.Policy{&policy.DeviceFlexHwDataForProductImprovementEnabled{Stat: policy.StatusSet, Val: false}},
			hwisNotAuthorizedResponse,
		},
	} {
		s.Run(ctx, test.name, func(ctx context.Context, s *testing.State) {
			if len(test.policies) > 0 {
				// For all environment settings, refer to the test of the device
				// policy report, such as /cros/policy/display_reporting_dbus.go
				s.Log("Set up the environment to enable the relevant policies")
				cr := s.FixtValue().(chrome.HasChrome).Chrome()
				fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

				// Update policies.
				if err := policyutil.ServeAndVerify(ctx, fdms, cr, test.policies); err != nil {
					s.Fatal("Failed to update policies: ", err)
				}
			}

			s.Log("Wait for background flex_hwis job to finish")
			// Flex device periodically runs `flex_hwis` command in background by upstart.　This
			// test might be ran at the same timing of the pediodical run.
			// We use TolerateWrongGoal because the job might be running (StartGoal) when we check,
			// and we want to wait until it finishes and transitions to StopGoal/WaitingState.
			if err := upstart.WaitForJobStatus(ctx, "flex_hwis", commonupstart.StopGoal, commonupstart.WaitingState, upstart.TolerateWrongGoal, 30*time.Second); err != nil {
				s.Log("Warning: flex_hwis job did not finish, proceeding anyway: ", err)
			}

			s.Log("Run and test the HWIS service")
			// The flex_hwis_tool service will check the /var/lib/flex_hwis_tool/time
			// file before running and make sure that it has not been run within the
			// specified time. To ensure that the service can run, remove possible
			// time file before running the command.
			os.Remove(hwisTimeFileName)
			out, err := testexec.CommandContext(ctx, "flex_hwis_tool", "--debug", "--send").CombinedOutput()
			// The flex_hwis_tool service will create a file to record the time after
			// successfully running. The service will not run again within the specified
			// time period. To ensure the test can run at any time, the time file must be
			// removed after the test is complete.
			os.Remove(hwisTimeFileName)

			outString := string(out)
			if err != nil {
				s.Fatalf("flex_hwis_tool fails to run and outputs: %s", outString)
			}
			if !bytes.Contains(out, []byte(test.expectedResponse)) {
				s.Fatalf("flex_hwis_tool doesn't contain expected string. Expected: %s, Output: %s", test.expectedResponse, outString)
			}
		})
	}
}
