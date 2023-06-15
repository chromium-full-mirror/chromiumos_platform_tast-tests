// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

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

const releaseChannelE2ETimeout = 4 * time.Minute
const omahaUpdateE2ETimeout = 15 * time.Minute

type testParam struct {
	ReleaseChannelPolicy tape.ReleaseChannelWithLtsEnum
	ExpectedPolicies     []policy.Policy
	expectedParameters   []string

	testOmaha               bool
	expectedLSBReleaseRegex map[string]string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReleaseChannelE2E,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Enroll a device in an OU and select a channel",
		Contacts: []string{
			"vsavu@google.com",          // Test author
			"crisguerrero@chromium.org", // Test author
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
		},
		Params: []testing.Param{{
			Name:    "lts",
			Timeout: releaseChannelE2ETimeout,
			Val: testParam{
				ReleaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_LTS_CHANNEL,
				ExpectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
					&policy.DeviceReleaseLtsTag{Stat: policy.StatusSet, Val: "lts"},
					// TODO(b/272761788):  Current LTS milestone is 108. Ensure
					// DeviceTargetVersionPrefix needs to be updated.
					&policy.DeviceTargetVersionPrefix{Stat: policy.StatusSet, Val: "15183."},
				},
				expectedParameters: []string{"targetversionprefix=\"15183.\"", "ltstag=\"lts\"", "track=\"stable-channel\""},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Release channel" in Admin Console and ensure that
				// ChromeOsReleaseChannel policy propagates to the device.
				// channel=lts
				// COM_FOUND_CUJ11_TASK3_WF3.
				Value: "screenplay-a1bdf4ff-f14d-43a5-90ec-17a4bd2cb397",
			}},
		}, {
			Name:    "stable",
			Timeout: releaseChannelE2ETimeout,
			Val: testParam{
				ReleaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_STABLE_CHANNEL,
				ExpectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
				},
				expectedParameters: []string{"track=\"stable-channel\""},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Release channel" in Admin Console and ensure that
				// ChromeOsReleaseChannel policy propagates to the device.
				// channel=stable
				// COM_FOUND_CUJ11_TASK3_WF2.
				Value: "screenplay-54300da1-e6b1-4664-a175-ff3fc644a17b",
			}},
		}, {
			Name:    "beta",
			Timeout: releaseChannelE2ETimeout,
			Val: testParam{
				ReleaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_BETA_CHANNEL,
				ExpectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "beta-channel"},
				},
				expectedParameters: []string{"track=\"beta-channel\""},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Release channel" in Admin Console and ensure that
				// ChromeOsReleaseChannel policy propagates to the device.
				// channel=beta
				// COM_FOUND_CUJ11_TASK3_WF1.
				Value: "screenplay-cbd3450e-2c0e-4da8-9d55-8977a34e5e4b",
			}},
		}, {
			Name:    "lts_omaha",
			Timeout: omahaUpdateE2ETimeout + releaseChannelE2ETimeout,
			Val: testParam{
				ReleaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_LTS_CHANNEL,
				ExpectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
					&policy.DeviceReleaseLtsTag{Stat: policy.StatusSet, Val: "lts"},
					// TODO(b/272761788):  Current LTS milestone is 108. Ensure
					// DeviceTargetVersionPrefix needs to be updated.
					&policy.DeviceTargetVersionPrefix{Stat: policy.StatusSet, Val: "15183."},
				},
				expectedParameters: []string{"targetversionprefix=\"15183.\"", "ltstag=\"lts\"", "track=\"stable-channel\""},

				testOmaha: true,
				expectedLSBReleaseRegex: map[string]string{
					"CHROMEOS_RELEASE_TRACK":   "^stable-channel$",
					"CHROMEOS_RELEASE_VERSION": "^15183[.].+[.].+$",
				},
			},
		}, {
			Name:    "stable_omaha",
			Timeout: omahaUpdateE2ETimeout + releaseChannelE2ETimeout,
			Val: testParam{
				ReleaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_STABLE_CHANNEL,
				ExpectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
				},
				expectedParameters: []string{"track=\"stable-channel\""},
				testOmaha:          true,
				expectedLSBReleaseRegex: map[string]string{
					"CHROMEOS_RELEASE_TRACK": "^stable-channel$",
				},
			},
		}, {
			Name:    "beta_omaha",
			Timeout: omahaUpdateE2ETimeout + releaseChannelE2ETimeout,
			Val: testParam{
				ReleaseChannelPolicy: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_BETA_CHANNEL,
				ExpectedPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "beta-channel"},
				},
				expectedParameters: []string{"track=\"beta-channel\""},
				testOmaha:          true,
				expectedLSBReleaseRegex: map[string]string{
					"CHROMEOS_RELEASE_TRACK": "^beta-channel$",
				},
			},
		}},
	})
}

func ReleaseChannelE2E(ctx context.Context, s *testing.State) {
	param := s.Param().(testParam)

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

	timeout := int32(releaseChannelE2ETimeout.Seconds())
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
		AutoUpdateTargetVersionLts: tape.AutoUpdateTargetVersionLts{
			SelectedVersion: tape.SelectedVersion{
				DisplayName: "No restriction",
			},
		},
		// Set target channel.
		ReleaseChannelWithLts: param.ReleaseChannelPolicy,
	}

	// Set autoupdate policy.
	if err := tapeClient.SetPolicy(ctx, tapePolicy, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, acc.RequestID); err != nil {
		s.Fatal("Failed to set the release channel: ", err)
	}

	dmServerURL := "https://crosman-alpha.sandbox.google.com/devicemanagement/data/api"
	if _, err := policyClient.GAIAEnrollAndLoginUsingChrome(ctx, &pspb.GAIAEnrollAndLoginUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: dmServerURL,
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

	pJSON, err := policy.MarshalList(param.ExpectedPolicies)
	if err != nil {
		s.Fatal("Error while marshalling policies to JSON: ", err)
	}

	// It may take some time to propagate the policy. Wait to avoid flakiness.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := policyClient.VerifyPolicyStatus(ctx, &pspb.VerifyPolicyStatusRequest{
			Policies: pJSON,
		})
		return err
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Error("Failed to verify policy: ", err)
	}

	if err := update.TriggerUpdateAndCheckNebraskaLogs(ctx, cl, param.expectedParameters); err != nil {
		s.Error("Failed to verify update reqeust: ", err)
	}

	if param.testOmaha {
		if err := update.PerformUpdateAndCheckImage(ctx, cl, param.expectedLSBReleaseRegex); err != nil {
			s.Error("Failed to update device: ", err)
		}
	}
}
