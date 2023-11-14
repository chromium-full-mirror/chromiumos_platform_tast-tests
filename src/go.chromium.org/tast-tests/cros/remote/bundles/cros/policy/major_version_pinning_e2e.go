// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/policy/update"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	aupb "go.chromium.org/tast-tests/cros/services/cros/autoupdate"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const majorVersionPinningE2ETimeout = 5 * time.Minute
const pinningOmahaUpdateE2ETimeout = 15 * time.Minute

type majorVersionPinningE2ETestParam struct {
	releaseChannelPolicy tape.ReleaseChannelWithLtsEnum
	targetMilestone      string
	expectedPolicies     []policy.Policy
	expectedParameters   []string

	testOmaha               bool
	expectedLSBReleaseRegex map[string]string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MajorVersionPinningE2E,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Configures a milestone to pin to and verifies a pinned major version value on a device",
		Contacts: []string{
			"artyomchen@google.com", // Test author
			"chromeos-commercial-remote-management@google.com",
		},
		BugComponent: "b:1031231",
		Attr:         []string{"group:golden_tier"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
			"tast.cros.tape.Service",
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ChromeOsReleaseChannel{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.DeviceReleaseLtsTag{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.DeviceTargetVersionPrefix{}, pci.VerifiedValue),
			{
				Key: "feature_id",
				// Configure "Target version" in "Auto-update settings" in Admin Console
				// and ensure that devices correctly update to the configured version.
				// COM_FOUND_CUJ13_TASK3_WF1
				Value: "screenplay-04b1d4dc-1363-46f5-a453-7937fae60bba",
			},
		},
		Fixture: fixture.UpdateEngine, // Ensures update engine is ready and resets its status.
		Params: []testing.Param{{
			Name:    "lts",
			Timeout: majorVersionPinningE2ETimeout,
			Val: majorVersionPinningE2ETestParam{
				releaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_LTS_CHANNEL,
				targetMilestone:      "114.* (long-term support)",
				expectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
					&policy.DeviceReleaseLtsTag{Stat: policy.StatusSet, Val: "lts"},
					&policy.DeviceTargetVersionPrefix{Stat: policy.StatusSet, Val: "15437."},
				},
				expectedParameters: []string{"targetversionprefix=\"15437.\"", "ltstag=\"lts\""},
			},
		}, {
			Name:    "stable",
			Timeout: majorVersionPinningE2ETimeout,
			Val: majorVersionPinningE2ETestParam{
				releaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_STABLE_CHANNEL,
				targetMilestone:      "115.*",
				expectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
					&policy.DeviceTargetVersionPrefix{Stat: policy.StatusSet, Val: "15474."},
				},
				expectedParameters: []string{"targetversionprefix=\"15474.\""},
			},
		}, {
			Name:    "lts_omaha",
			Timeout: majorVersionPinningE2ETimeout + pinningOmahaUpdateE2ETimeout,
			Val: majorVersionPinningE2ETestParam{
				releaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_LTS_CHANNEL,
				targetMilestone:      "114.* (long-term support)",
				expectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
					&policy.DeviceReleaseLtsTag{Stat: policy.StatusSet, Val: "lts"},
					&policy.DeviceTargetVersionPrefix{Stat: policy.StatusSet, Val: "15437."},
				},
				expectedParameters: []string{"targetversionprefix=\"15437.\"", "ltstag=\"lts\""},
				testOmaha:          true,
				expectedLSBReleaseRegex: map[string]string{
					"CHROMEOS_RELEASE_TRACK":   "^stable-channel$",
					"CHROMEOS_RELEASE_VERSION": "^15437[.].+[.].+$",
				},
			},
		}, {
			Name:    "stable_omaha",
			Timeout: majorVersionPinningE2ETimeout + pinningOmahaUpdateE2ETimeout,
			Val: majorVersionPinningE2ETestParam{
				releaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_STABLE_CHANNEL,
				targetMilestone:      "115.*",
				expectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
					&policy.DeviceTargetVersionPrefix{Stat: policy.StatusSet, Val: "15474."},
				},
				expectedParameters: []string{"targetversionprefix=\"15474.\""},
				testOmaha:          true,
				expectedLSBReleaseRegex: map[string]string{
					"CHROMEOS_RELEASE_TRACK":   "^stable-channel$",
					"CHROMEOS_RELEASE_VERSION": "^15474[.].+[.].+$",
				},
			},
		}},
	})
}

func MajorVersionPinningE2E(ctx context.Context, s *testing.State) {
	param := s.Param().(majorVersionPinningE2ETestParam)

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(cleanupCtx)

	if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(majorVersionPinningE2ETimeout.Seconds())
	poolID := tape.DefaultManaged
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	tapePolicy := &tape.AutoUpdateSettingsDevices{
		// Set required fields for API.
		UpdateDisabled:                  false,
		AutoUpdateAllowedConnectionType: tape.AUTOUPDATECONNECTIONTYPEENUM_AUTO_UPDATE_CONNECTION_TYPE_ENUM_ALL_CONNECTIONS,
		DeviceRollbackToTargetVersion:   1, // Roll back OS disabled.
		AutoUpdateRolloutPlan: tape.AutoUpdateRolloutPlan{
			Plan:    tape.ROLLOUTPLAN_SCATTER_UPDATES,
			Scatter: tape.SCATTERFACTOR_ONE_DAY,
		},
		// Set target channel.
		ReleaseChannelWithLts: param.releaseChannelPolicy,
		// Set milestone to pin to.
		AutoUpdateTargetVersionLts: tape.AutoUpdateTargetVersionLts{
			SelectedVersion: tape.SelectedVersion{
				DisplayName: param.targetMilestone,
			},
		},
	}

	// Set autoupdate policy.
	if err := tapeClient.SetPolicy(ctx, tapePolicy, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, acc.RequestID); err != nil {
		s.Fatal("Failed to set the release channel: ", err)
	}

	if _, err := policyClient.GAIAEnrollAndLoginUsingChrome(ctx, &pspb.GAIAEnrollAndLoginUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: policy.DMServerAlphaURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(ctx, &empty.Empty{})

	// Deprovision the DUT at the end of the test.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(cleanupCtx, cl, acc.CustomerID, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	// Restart update_engine after enrollment.
	updateClient := aupb.NewUpdateServiceClient(cl.Conn)
	if _, err := updateClient.ResetUpdateEngine(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to reset update_engine: ", err)
	}

	pJSON, err := policy.MarshalList(param.expectedPolicies)
	if err != nil {
		s.Fatal("Error while marshalling policies to JSON: ", err)
	}

	// It may take some time to propagate the policy. Wait to avoid flakiness.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := policyClient.VerifyPolicyStatus(ctx, &pspb.VerifyPolicyStatusRequest{
			Policies: pJSON,
		})
		return err
	}, &testing.PollOptions{Timeout: 1 * time.Minute}); err != nil {
		s.Error("Failed to verify policy: ", err)
	}

	if err := update.TriggerUpdateAndCheckNebraskaLogs(ctx, cl, param.expectedParameters); err != nil {
		s.Error("Failed to verify update request: ", err)
	}

	if param.testOmaha {
		if err := update.PerformUpdateAndCheckImage(ctx, cl, param.expectedLSBReleaseRegex); err != nil {
			s.Error("Failed to update device: ", err)
		}
	}
}
