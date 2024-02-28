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
	"go.chromium.org/tast-tests/cros/local/metrics"
	"go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/rollback"
	"go.chromium.org/tast-tests/cros/local/vpd"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type updateEngineTestParam struct {
	// policyValues are the policies that need to be set.
	policyValues []policy.Policy
	// expectedUpdateRequestSubstrings is a list of substrings that should be present
	// in an update request with the given policies set.
	expectedUpdateRequestSubstrings []string
	// generateExpectedUpdateRequestSubstrings generates strings to be added to
	// expectedUpdateRequestSubstrings. Use for strings that can only be obtained at runtime.
	generateExpectedUpdateRequestSubstrings func() ([]string, error)

	// excludedUpdateRequestSubstrings is a list of substrings that are not supposed to be present
	// in an update request with given policies.
	excludedUpdateRequestSubstrings []string

	// verifyRollbackMetricsFile is only need for Enterprise Rollback to ensure the
	// rollback metrics file is on sync with the policy.
	verifyRollbackMetricsFile bool
}

const (
	deviceTargetVersionPrefixVal = "1000."
	deviceReleaseLtsTagVal       = "lts"
	rollbackAndRestoreIfPossible = 3
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
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChromeUpdateEngineEnrolledLoggedIn,
		Timeout:      1 * time.Minute,
		Params: []testing.Param{{
			Name: "device_target_version_prefix",
			Val: &updateEngineTestParam{
				policyValues:                    []policy.Policy{&policy.DeviceTargetVersionPrefix{Val: deviceTargetVersionPrefixVal}},
				expectedUpdateRequestSubstrings: []string{fmt.Sprintf("targetversionprefix=%q", deviceTargetVersionPrefixVal)},
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
				policyValues:                    []policy.Policy{&policy.DeviceReleaseLtsTag{Val: deviceReleaseLtsTagVal}},
				expectedUpdateRequestSubstrings: []string{fmt.Sprintf("ltstag=%q", deviceReleaseLtsTagVal)},
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
					&policy.DeviceRollbackToTargetVersion{Val: rollbackAndRestoreIfPossible},
					// Metrics reporting must be enabled to ensure rollback metrics file is created with the rollback policy.
					&policy.DeviceMetricsReportingEnabled{Stat: policy.StatusSetRecommended, Val: true},
				},
				verifyRollbackMetricsFile: true,
				expectedUpdateRequestSubstrings: []string{
					"rollback_allowed=\"true\"",
					fmt.Sprintf("targetversionprefix=%q", deviceTargetVersionPrefixVal)},
				generateExpectedUpdateRequestSubstrings: rollbackExptectedSubstrings,
			},
		}, {
			Name: "device_channel_stable",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Val: "stable-channel"},
					&policy.ChromeOsReleaseChannelDelegated{Val: false},
				},
				expectedUpdateRequestSubstrings: []string{"track=\"stable-channel\""},
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
				expectedUpdateRequestSubstrings: []string{"track=\"beta-channel\""},
			},
		}, {
			Name: "device_channel_dev",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Val: "dev-channel"},
					&policy.ChromeOsReleaseChannelDelegated{Val: false},
				},
				expectedUpdateRequestSubstrings: []string{"track=\"dev-channel\""},
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
				expectedUpdateRequestSubstrings: []string{"track=\"ltc-channel\""},
			},
		}, {
			Name: "device_channel_lts",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Val: "lts-channel"},
					&policy.ChromeOsReleaseChannelDelegated{Val: false},
				},
				expectedUpdateRequestSubstrings: []string{"track=\"lts-channel\""},
			},
		}, {
			Name: "no_policy",
			Val: &updateEngineTestParam{
				policyValues:                    []policy.Policy{},
				expectedUpdateRequestSubstrings: []string{"Sent response"},
				excludedUpdateRequestSubstrings: []string{
					"targetversionprefix",
					"ltstag",
					"rollback_allowed",
					"activate_date",
					"fsi_version",
				},
			},
		}, {
			Name: "quick_fix_build",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.DeviceQuickFixBuildToken{Val: "testToken"},
				},
				expectedUpdateRequestSubstrings: []string{"cohorthint=\"testToken\""},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// After DeviceQuickFixBuildToken has been configured using
				// go/cros-qfb-release#using-engtoolservice, verify that the
				// devices update to the corresponding ChromeOS version.
				// COM_FOUND_CUJ21_TASK3_WF1
				Value: "screenplay-50b385e7-4bd5-4ee3-bf3e-f49c6f679540",
			}},
		}, {
			Name: "extended_auto_updates_enabled",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.DeviceExtendedAutoUpdateEnabled{Val: true},
				},
				expectedUpdateRequestSubstrings: []string{"extended_okay=\"true\""},
			},
		}, {
			Name: "extended_auto_updates_disabled",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.DeviceExtendedAutoUpdateEnabled{Val: false},
				},
				expectedUpdateRequestSubstrings: []string{"extended_okay=\"false\""},
			},
		}, {
			Name: "extended_auto_updates_unset",
			Val: &updateEngineTestParam{
				policyValues: []policy.Policy{
					&policy.DeviceExtendedAutoUpdateEnabled{},
				},
				expectedUpdateRequestSubstrings: []string{"extended_okay=\"false\""},
			},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceReleaseLtsTag{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceRollbackToTargetVersion{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceTargetVersionPrefix{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ChromeOsReleaseChannel{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ChromeOsReleaseChannelDelegated{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceMetricsReportingEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceQuickFixBuildToken{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceExtendedAutoUpdateEnabled{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func UpdateEnginePolicies(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	param := s.Param().(*updateEngineTestParam)

	defer policyutil.ServeAndVerify(cleanupCtx, fdms, cr, []policy.Policy{})

	updateServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
	if err != nil {
		s.Fatal("Failed to start nebraska: ", err)
	}
	defer updateServer.Close(cleanupCtx)

	if param.verifyRollbackMetricsFile {
		if err := rollback.CheckFileDoesNotExist(rollback.MetricsData); err != nil {
			s.Fatal("Failure when checking the rollback metrics file does not exist if policy is disabled: ", err)
		}
	}

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

	expectedSubstrings := param.expectedUpdateRequestSubstrings
	if param.generateExpectedUpdateRequestSubstrings != nil {
		generatedExpectedSubstrings, err := param.generateExpectedUpdateRequestSubstrings()
		if err != nil {
			s.Error("Failed to compute expected update request substring: ", err)
		}
		expectedSubstrings = append(expectedSubstrings, generatedExpectedSubstrings...)
	}

	if err := awaitUpdateServerLogEntries(ctx, updateServer, expectedSubstrings); err != nil {
		s.Error("Failure while waiting for update logs: ", err)
	}

	updateServerLog, err := os.ReadFile(updateServer.LogFile)
	if err != nil {
		s.Fatal("Failed to read nebraska logs: ", err)
	}

	for _, entry := range param.excludedUpdateRequestSubstrings {
		if strings.Contains(string(updateServerLog), entry) {
			s.Errorf("Unexpectedly found %q in nebraska logs", entry)
		}
	}

	if param.verifyRollbackMetricsFile {
		if err := verifyRollbackMetricsFileSyncsWithPolicy(ctx, fdms, cr); err != nil {
			s.Fatal("Rollback metrics file not in sync with rollback policy: ", err)
		}
	}
}

func verifyRollbackMetricsFileSyncsWithPolicy(ctx context.Context, fdms *fakedms.FakeDMS, cr *chrome.Chrome) error {
	// Metrics consent must be enabled because of DeviceMetricsReportingEnabled.
	if consent, err := metrics.HasConsent(ctx); err != nil {
		return errors.Wrap(err, "failed to check metrics consent status")
	} else if !consent {
		return errors.New("Consent metrics is disabled")
	}

	if err := rollback.CheckFileExists(rollback.MetricsData); err != nil {
		return errors.Wrap(err, "rollback metrics file missing despite policy")
	}
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{}); err != nil {
		return errors.Wrap(err, "failed to restore default policies")
	}
	if err := rollback.CheckFileDoesNotExist(rollback.MetricsData); err != nil {
		return errors.Wrap(err, "rollback metrics file not deleted despite policy change")
	}

	return nil
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

// awaitUpdateServerLogEntries waits for all of the given entries to show up in nebraska logs.
func awaitUpdateServerLogEntries(ctx context.Context, updateServer *nebraska.Nebraska, entries []string) error {
	testing.ContextLogf(ctx, "Waiting for %q to show up in nebraska logs", entries)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		updateServerLog, err := os.ReadFile(updateServer.LogFile)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read nebraska logs"))
		}

		for _, entry := range entries {
			if !strings.Contains(string(updateServerLog), entry) {
				return errors.Errorf("%q not in the nebraska logs", entry)
			}
		}

		return nil
	}, &testing.PollOptions{
		Timeout: 20 * time.Second,
	}); err != nil {
		return errors.Wrapf(err, "could not find all of %q in nebraska logs", entries)
	}
	return nil
}

func rollbackExptectedSubstrings() ([]string, error) {
	activateDate, err := vpd.ActivateDate()
	if err != nil {
		return []string{}, errors.Wrap(err, "failed to read activate date")
	}
	versionEntry := "activate_date=\"" + *activateDate + "\""

	fsiVersion, err := vpd.FsiVersion()
	if err != nil {
		return []string{}, errors.Wrap(err, "failed to read fsi version")
	}
	if fsiVersion != nil {
		// FSI version takes precedence.
		versionEntry = "fsi_version=\"" + *fsiVersion + "\""
	}

	return []string{versionEntry}, nil
}
