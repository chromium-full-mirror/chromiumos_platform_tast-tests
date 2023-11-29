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
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const devicePolicyPropagationTimeE2ETimeout = 25 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevicePolicyPropagationTimeE2E,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "E2E test for testing that updated device policies propagate from DPanel to a device within 10 minutes",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"artyomchen@google.com", // Test author
		},
		BugComponent: "b:1111617", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Policy Stack
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
			"tast.cros.tape.Service",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceAutoUpdateDisabled{}, pci.Served),
			{
				Key: "feature_id",
				// As an admin, I want to ensure that right policies are
				// propagated to my managed devices, so I verify that
				// the device has received updated policies in
				// under 10 minutes.
				// TODO(b/308932300): Integrate crosbolt.
				Value: "screenplay-b0bb6119-2d9c-4e04-810e-bb4b83b96889",
			},
		},
		Fixture: fixture.CleanOwnership,
		Timeout: devicePolicyPropagationTimeE2ETimeout,
	})
}

func DevicePolicyPropagationTimeE2E(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	// Create an account manager and lease a test account for the duration of the test.
	tapeTimeout := int32(devicePolicyPropagationTimeE2ETimeout.Seconds())
	poolID := tape.DefaultManaged
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(tapeTimeout), tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)
	// Deprovision the DUT at the end of the test.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(cleanupCtx, cl, acc.CustomerID, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	// Set initial device policies.
	initialTapePolicy := &tape.AutoUpdateSettingsDevices{
		UpdateDisabled: true,
	}
	testing.ContextLog(ctx, "Setting initial policies via tape")
	if err := tapeClient.SetPolicy(ctx, initialTapePolicy, []string{"updateDisabled"} /*updateMask*/, nil /*additionalTargetKeys*/, acc.RequestID); err != nil {
		s.Fatal("Failed to set initial policy: ", err)
	}

	if _, err := policyClient.GAIAEnrollAndLoginUsingChrome(ctx, &pspb.GAIAEnrollAndLoginUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: policy.DMServerAlphaURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(cleanupCtx, &empty.Empty{})

	// Verify initial device policies.
	expectedInitialPolicy := []policy.Policy{
		&policy.DeviceAutoUpdateDisabled{Stat: policy.StatusSet, Val: true},
	}
	pJSON, err := policy.MarshalList(expectedInitialPolicy)
	if err != nil {
		s.Fatal("Error while marshalling initial policies to JSON: ", err)
	}
	testing.ContextLog(ctx, "Verifying initial policies")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := policyClient.VerifyPolicyStatus(ctx, &pspb.VerifyPolicyStatusRequest{
			Policies: pJSON,
		})
		return err
	}, &testing.PollOptions{Timeout: 10 * time.Minute}); err != nil {
		s.Fatal("Failed to verify initial policy: ", err)
	}

	// Set new device policies.
	tapePolicy := &tape.AutoUpdateSettingsDevices{
		UpdateDisabled: false,
		// Set required fields for API.
		AutoUpdateAllowedConnectionType: tape.AUTOUPDATECONNECTIONTYPEENUM_AUTO_UPDATE_CONNECTION_TYPE_ENUM_ALL_CONNECTIONS,
		DeviceRollbackToTargetVersion:   1, // Roll back OS disabled.
		AutoUpdateRolloutPlan: tape.AutoUpdateRolloutPlan{
			Plan:    tape.ROLLOUTPLAN_SCATTER_UPDATES,
			Scatter: tape.SCATTERFACTOR_ONE_DAY,
		},
		ReleaseChannelWithLts: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_STABLE_CHANNEL,
		AutoUpdateTargetVersionLts: tape.AutoUpdateTargetVersionLts{
			SelectedVersion: tape.SelectedVersion{
				DisplayName: "No restriction",
			},
		},
	}
	testing.ContextLog(ctx, "Setting new policies via tape")
	if err := tapeClient.SetPolicy(ctx, tapePolicy, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, acc.RequestID); err != nil {
		s.Fatal("Failed to set new policy: ", err)
	}

	// Verify new device policies.
	expectedPolicy := []policy.Policy{
		&policy.DeviceAutoUpdateDisabled{Stat: policy.StatusSet, Val: false},
	}
	pJSON, err = policy.MarshalList(expectedPolicy)
	if err != nil {
		s.Fatal("Error while marshalling new policies to JSON: ", err)
	}
	testing.ContextLog(ctx, "Verifying new policies via tape")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := policyClient.VerifyPolicyStatus(ctx, &pspb.VerifyPolicyStatusRequest{
			Policies: pJSON,
		})
		return err
	}, &testing.PollOptions{Timeout: 10 * time.Minute}); err != nil {
		s.Fatal("Failed to verify new policy: ", err)
	}
}
