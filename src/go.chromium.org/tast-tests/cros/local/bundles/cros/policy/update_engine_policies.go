// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type updateEngineTestParam struct {
	// policyValues are the policies that need to be set.
	policyValues []policy.Policy
	// policyParam is the xml attribute that needs to be set by update_engine.
	policyParam string
	// testValue is the value for the policyParam attribute.
	testValue string

	// Some values are too generic or are always set, allow skipping the check when the policies are unset.
	// checkParam indicates whether to check for the xml attribute.
	checkParam bool
	// checkVal indicates whether to check for the value.
	checkVal bool
}

const (
	deviceTargetVersionSelectorVal = "0,1626155736-"
	deviceTargetVersionPrefixVal   = "1000."
	deviceReleaseLtsTagVal         = "lts"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UpdateEnginePolicies,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check of policies are properly propagating to update_engine by checking the logs",
		BugComponent: "b:1031231",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test author
		},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.ChromeUpdateEngineEnrolledLoggedIn,
		Timeout:      1 * time.Minute,
		Params: []testing.Param{{
			Name: "device_target_version_selector",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{&policy.DeviceTargetVersionSelector{Val: deviceTargetVersionSelectorVal}},
				testValue:    deviceTargetVersionSelectorVal,
				policyParam:  "targetversionselector",
				checkParam:   true,
				checkVal:     true,
			},
		}, {
			Name: "device_target_version_prefix",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{&policy.DeviceTargetVersionPrefix{Val: deviceTargetVersionPrefixVal}},
				testValue:    deviceTargetVersionPrefixVal,
				policyParam:  "targetversionprefix",
				checkParam:   true,
				checkVal:     true,
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Target version" in Admin Console and ensure that affected devices stay on selected version.
				// COM_FOUND_CUJ12_TASK4_WF1
				Value: "screenplay-5f27f0ec-9865-4b66-babe-4114811d2617",
			}, {
				Key: "feature_id",
				// Can pin the OS version.
				// COM_KIOSK_CUJ3_TASK2_WF1
				Value: "screenplay-716d9d83-9b88-4034-94d3-0a4760bc835a",
			}},
		}, {
			Name: "device_release_lts_tag",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{&policy.DeviceReleaseLtsTag{Val: deviceReleaseLtsTagVal}},
				testValue:    deviceReleaseLtsTagVal,
				policyParam:  "ltstag",
				checkParam:   true,
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Set ChromeOsReleaseChannel policy on the device locally via FakeDMS and ensure that updates work as expected.
				// channel-from-to=stable-to-lts
				// COM_FOUND_CUJ11_TASK5_WF4
				Value: "screenplay-6e042833-6078-4ca2-ae09-ff808c5db446",
			}},
		}, {
			Name: "device_rollback_to_target_version",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.DeviceTargetVersionPrefix{Val: deviceTargetVersionPrefixVal},
					&policy.DeviceRollbackToTargetVersion{Val: 2},
				},
				testValue:   "true",
				policyParam: "rollback_allowed",
			},
		}, {
			Name: "device_channel_stable",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Val: "stable-channel"},
					&policy.ChromeOsReleaseChannelDelegated{Val: false},
				},
				testValue:   "stable-channel",
				policyParam: "track",
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Set ChromeOsReleaseChannel policy on the device locally via FakeDMS and ensure that updates work as expected.
				// channel-from-to=beta-to-stable
				// COM_FOUND_CUJ11_TASK5_WF1
				Value: "screenplay-d3df997c-ae2f-471e-9d3b-b33d8df6cb92",
			}, {
				Key: "feature_id",
				// Set ChromeOsReleaseChannel policy on the device locally via FakeDMS and ensure that updates work as expected.
				// channel-from-to=ltc-to-stable
				// COM_FOUND_CUJ11_TASK5_WF3
				Value: "screenplay-fdbbf9e6-564f-4a6c-97d3-43c899c5d2e4",
			}},
		}, {
			Name: "device_channel_beta",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Val: "beta-channel"},
					&policy.ChromeOsReleaseChannelDelegated{Val: false},
				},
				testValue:   "beta-channel",
				policyParam: "track",
			},
		}, {
			Name: "device_channel_dev",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Val: "dev-channel"},
					&policy.ChromeOsReleaseChannelDelegated{Val: false},
				},
				testValue:   "dev-channel",
				policyParam: "track",
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Set ChromeOsReleaseChannel policy on the device locally via FakeDMS and ensure that updates work as expected.
				// channel-from-to=stable-to-dev
				// COM_FOUND_CUJ11_TASK5_WF2
				Value: "screenplay-ffe64e90-9827-4dde-8f66-ac0143a9b71b",
			}},
		}, {
			Name: "device_channel_ltc",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Val: "ltc-channel"},
					&policy.ChromeOsReleaseChannelDelegated{Val: false},
				},
				testValue:   "ltc-channel",
				policyParam: "track",
			},
		}, {
			Name: "device_channel_lts",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Val: "lts-channel"},
					&policy.ChromeOsReleaseChannelDelegated{Val: false},
				},
				testValue:   "lts-channel",
				policyParam: "track",
			},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceTargetVersionSelector{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceReleaseLtsTag{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceRollbackToTargetVersion{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceTargetVersionPrefix{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ChromeOsReleaseChannel{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ChromeOsReleaseChannelDelegated{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// triggerUpdate requests an update check at the specified Omaha URL.
func triggerUpdate(ctx context.Context, url string) error {
	// Make sure update_engine_client does not hang.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := testexec.CommandContext(ctx, "update_engine_client", "--check_for_update", fmt.Sprintf("--omaha_url=%s", url)).Run(testexec.DumpLogOnError); err != nil {
		return err
	}

	return nil
}

func UpdateEnginePolicies(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	param := s.Param().(*updateEngineTestParam)

	defer policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{})

	const waitTime = 20 * time.Second

	s.Run(ctx, "set", func(ctx context.Context, s *testing.State) {
		updateServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
		if err != nil {
			s.Fatal("Failed to start nebraska: ", err)
		}
		defer updateServer.Close(ctx)

		// Set the policy and check that the attribute is set.
		// TODO(b/285292962): Replace poll with a test-agnostic workaround.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return policyutil.ServeAndVerify(ctx, fdms, cr, param.policyValues)
		}, &testing.PollOptions{
			Timeout: 30 * time.Second,
		}); err != nil {
			s.Fatal("Failed to update policies: ", err)
		}

		if err := triggerUpdate(ctx, nebraska.UpdateURL(updateServer.Port, true)); err != nil {
			s.Fatal("Failed to trigger update request: ", err)
		}

		attributeEntry := param.policyParam + "=\"" + param.testValue + "\""
		s.Log("Waiting for the log entry to show up")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var err error
			updateServerLog, err := os.ReadFile(updateServer.LogFile)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to read nebraska logs"))
			}

			if !strings.Contains(string(updateServerLog), attributeEntry) {
				return errors.Errorf("%q not in the nebraska logs", attributeEntry)
			}

			return nil
		}, &testing.PollOptions{
			Timeout: waitTime,
		}); err != nil {
			s.Error("Could not find expected values: ", err)
		}
	})

	s.Run(ctx, "unset", func(ctx context.Context, s *testing.State) {
		updateServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
		if err != nil {
			s.Fatal("Failed to start nebraska: ", err)
		}
		defer updateServer.Close(ctx)

		// Clear policies to make sure attribute is not always sent.
		// TODO(b/285292962): Replace poll with a test-agnostic workaround.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{})
		}, &testing.PollOptions{
			Timeout: 30 * time.Second,
		}); err != nil {
			s.Fatal("Failed to update policies: ", err)
		}

		if err := triggerUpdate(ctx, nebraska.UpdateURL(updateServer.Port, true)); err != nil {
			s.Fatal("Failed to trigger update request: ", err)
		}

		s.Log("Waiting for the nebraska request to finish")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var err error
			updateServerLog, err := os.ReadFile(updateServer.LogFile)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to read nebraska logs"))
			}

			responseLog := "Sent response"
			if !strings.Contains(string(updateServerLog), responseLog) {
				return errors.Errorf("%q not in the nebraska logs", responseLog)
			}

			return nil
		}, &testing.PollOptions{
			Timeout: waitTime,
		}); err != nil {
			s.Error("Could not find expected values: ", err)
		}

		updateServerLog, err := os.ReadFile(updateServer.LogFile)
		if err != nil {
			s.Fatal("Failed to read nebraska logs: ", err)
		}

		if param.checkParam && strings.Contains(string(updateServerLog), param.policyParam) {
			s.Errorf("Unexpectedly found %q in the nebraska logs", param.policyParam)
		}

		if param.checkVal && strings.Contains(string(updateServerLog), param.testValue) {
			s.Errorf("Unexpectedly found test value %q in the nebraska logs", param.testValue)
		}
	})
}
