// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/policy/nebraska"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

type updateEngineTestParam struct {
	// policyValues are the policies that need to be set.
	policyValues []policy.Policy
	// policyParam os the xml attribute that needs to be set by update_engine.
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
		Fixture:      fixture.ChromeEnrolledLoggedIn,
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
				Key:   "feature_id",
				Value: "screenplay-5f27f0ec-9865-4b66-babe-4114811d2617",
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
				Key:   "feature_id",
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
				Key:   "feature_id",
				Value: "screenplay-d3df997c-ae2f-471e-9d3b-b33d8df6cb92",
			}, {
				Key:   "feature_id",
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
				Key:   "feature_id",
				Value: "screenplay-ffe64e90-9827-4dde-8f66-ac0143a9b71b",
			}},
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

const updateEngineLog = "/var/log/update_engine.log"
const waitTime = 10 * time.Second

// clearAndUpdate restarts update engine, clears the logs and requests an update.
func clearAndUpdate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	if err := upstart.StopJob(ctx, "update-engine"); err != nil {
		return errors.Wrap(err, "failed to stop update_engine")
	}

	realLog, err := os.Readlink(updateEngineLog)
	if err != nil {
		return errors.Wrap(err, "failed to find the real update_engine log")
	}

	if err := os.Remove(realLog); err != nil {
		return errors.Wrap(err, "failed to clear the real update_engine log")
	}

	if err := os.Remove(updateEngineLog); err != nil {
		return errors.Wrap(err, "failed to clear the update_engine log")
	}

	if err := upstart.StartJob(ctx, "update-engine"); err != nil {
		return errors.Wrap(err, "failed to start update_engine")
	}

	// update_engine is not ready right after start.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Make sure update_engine_client does not hang.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := testexec.CommandContext(ctx, "update_engine_client", "--check_for_update").Run(testexec.DumpLogOnError); err != nil {
			return err
		}

		return nil
	}, nil); err != nil {
		return errors.Wrap(err, "failed to trigger update check")
	}

	return nil
}

func UpdateEnginePolicies(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	param := s.Param().(*updateEngineTestParam)

	// Restart update-engine after clearing policies.
	defer upstart.RestartJob(ctx, "update-engine")
	defer policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{})

	s.Run(ctx, "set", func(ctx context.Context, s *testing.State) {
		updateServer, err := nebraska.Start(ctx)
		if err != nil {
			s.Fatal("Failed to start nebraska: ", err)
		}
		defer updateServer.Stop(ctx)

		if err := updateServer.ConfigureStatefulLSBRelease(); err != nil {
			s.Fatal("Failed to configure lsb-release: ", err)
		}

		// Set the policy and check that the attribute is set.
		if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.policyValues); err != nil {
			s.Fatal("Failed to update policies: ", err)
		}

		if err := clearAndUpdate(ctx); err != nil {
			s.Fatal("Failed to trigger update request: ", err)
		}

		attributeEntry := param.policyParam + "=\"" + param.testValue + "\""
		s.Log("Waiting for the log entry to show up")
		var updateServerLog []byte
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var err error
			updateServerLog, err := updateServer.ReadLog(ctx)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to read nebraska logs"))
			}

			if !strings.Contains(string(updateServerLog), attributeEntry) {
				return errors.Errorf("%q not in the update_engine logs", attributeEntry)
			}

			return nil
		}, &testing.PollOptions{
			Timeout: waitTime,
		}); err != nil {
			s.Error("Could not find expected values: ", err)
		}

		updateEngineLog, err := ioutil.ReadFile("/var/log/update_engine.log")
		if err != nil {
			s.Fatal("Failed to read update_engine logs: ", err)
		}

		if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "set_log.txt"), updateEngineLog, 0644); err != nil {
			s.Error("Failed to dump update_engine logs: ", err)
		}

		if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "set_nebraska_log.txt"), updateServerLog, 0644); err != nil {
			s.Error("Failed to dump nebraska logs: ", err)
		}
	})

	s.Run(ctx, "unset", func(ctx context.Context, s *testing.State) {
		updateServer, err := nebraska.Start(ctx)
		if err != nil {
			s.Fatal("Failed to start nebraska: ", err)
		}
		defer updateServer.Stop(ctx)

		if err := updateServer.ConfigureStatefulLSBRelease(); err != nil {
			s.Fatal("Failed to configure lsb-release: ", err)
		}

		// Clear policies to make sure attribute is not always sent.
		if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{}); err != nil {
			s.Fatal("Failed to clear policies: ", err)
		}

		if err := clearAndUpdate(ctx); err != nil {
			s.Fatal("Failed to trigger update request: ", err)
		}

		s.Log("Waiting for update_engine to have a chance to log")
		if err := testing.Sleep(ctx, waitTime); err != nil {
			s.Fatal("Failed to wait for messages: ", err)
		}

		updateEngineLog, err := ioutil.ReadFile("/var/log/update_engine.log")
		if err != nil {
			s.Fatal("Failed to read update_engine logs: ", err)
		}

		if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "unset_log.txt"), updateEngineLog, 0644); err != nil {
			s.Error("Failed to dump update_engine logs: ", err)
		}

		updateServerLog, err := updateServer.ReadLog(ctx)
		if err != nil {
			s.Fatal("Failed to read nebraska logs: ", err)
		}

		if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "unset_nebraska_log.txt"), updateServerLog, 0644); err != nil {
			s.Error("Failed to dump nebraska logs: ", err)
		}

		if param.checkParam && strings.Contains(string(updateServerLog), param.policyParam) {
			s.Errorf("Unexpectedly found %q in the nebraska logs", param.policyParam)
		}

		if param.checkVal && strings.Contains(string(updateServerLog), param.testValue) {
			s.Errorf("Unexpectedly found test value %q in the nebraska logs", param.testValue)
		}
	})
}
