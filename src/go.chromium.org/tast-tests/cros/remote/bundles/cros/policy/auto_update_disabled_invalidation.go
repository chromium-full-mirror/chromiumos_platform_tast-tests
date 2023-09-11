// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/updateutil"
	"go.chromium.org/tast-tests/cros/services/cros/autoupdate"
	"go.chromium.org/tast-tests/cros/services/cros/nebraska"
	policypkg "go.chromium.org/tast-tests/cros/services/cros/policy"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

type autoUpdateDisabledInvalidationTestParam struct {
	extraPolicies []policy.Policy
	isRollback    bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutoUpdateDisabledInvalidation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the update engine invalidates completed pending updates if updates are disabled by policy",
		Contacts: []string{
			"artyomchen@google.com", // Test author
			"chromeos-commercial-remote-management@google.com",
		},
		Fixture:      fixture.Autoupdate,
		BugComponent: "b:1031231", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Version Control
		Attr:         []string{"group:autoupdate"},
		SoftwareDeps: []string{"reboot", "chrome", "crossystem"},
		ServiceDeps: []string{
			"tast.cros.baserpc.FileSystem",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
		},
		Timeout: updateutil.UpdateTimeout + 10*time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceAutoUpdateDisabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.RebootAfterUpdate{}, pci.Served),
			pci.SearchFlag(&policy.ChromeOsReleaseChannel{}, pci.Served),
			pci.SearchFlag(&policy.DeviceTargetVersionPrefix{}, pci.Served),
			pci.SearchFlag(&policy.DeviceRollbackAllowedMilestones{}, pci.Served),
			pci.SearchFlag(&policy.DeviceRollbackToTargetVersion{}, pci.Served),
		},
		Params: []testing.Param{{
			Name: "stable",
			Val: autoUpdateDisabledInvalidationTestParam{
				extraPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
				},
				isRollback: false,
			},
		}, {
			Name: "rollback",
			Val: autoUpdateDisabledInvalidationTestParam{
				extraPolicies: []policy.Policy{
					&policy.ChromeOsReleaseChannel{Stat: policy.StatusSet, Val: "stable-channel"},
					&policy.DeviceTargetVersionPrefix{Val: "15532."},
					&policy.DeviceRollbackAllowedMilestones{Val: 4},
					&policy.DeviceRollbackToTargetVersion{Val: 3},
				},
				isRollback: true,
			},
		}},
	})
}

func AutoUpdateDisabledInvalidation(ctx context.Context, s *testing.State) {
	param := s.Param().(autoUpdateDisabledInvalidationTestParam)

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

	rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer func(ctx context.Context) {
		if rpcClient != nil {
			rpcClient.Close(ctx)
		}
	}(cleanupCtx)

	// Enroll the DUT and set the policy.
	pb := policy.NewBlob()
	// Enable autoupdates and reboot after updates explicitly, so that they do not conflict with the test.
	pb.AddPolicies([]policy.Policy{
		&policy.DeviceAutoUpdateDisabled{Val: false},
		&policy.RebootAfterUpdate{Val: false},
	})
	pb.AddPolicies(param.extraPolicies)
	pJSON, err := json.Marshal(pb)
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}

	policyClient := pspb.NewPolicyServiceClient(rpcClient.Conn)
	if _, err := policyClient.EnrollUsingChrome(ctx, &pspb.EnrollUsingChromeRequest{
		PolicyJson: pJSON,
	}); err != nil {
		s.Fatal("Failed to enroll: ", err)
	}
	defer func(ctx context.Context) {
		if rpcClient != nil {
			// Recreating since the client can have a stale RPC connection after the reboot.
			policyClient := pspb.NewPolicyServiceClient(rpcClient.Conn)
			if _, err := policyClient.StopChromeAndFakeDMS(ctx, &empty.Empty{}); err != nil {
				s.Error("Failed to stop chrome and fake dms: ", err)
			}
		}
	}(cleanupCtx)

	mainFWAct, err := updateutil.GetCrossystemFlag(ctx, s.DUT(), updateutil.MainFWActFlag)
	if err != nil {
		s.Fatal("Failed to get current firmware partition: ", err)
	}

	// Read the current builder path from the lsb-release file for the in-place update.
	lsbContent := map[string]string{
		lsbrelease.BuilderPath: "",
	}
	if err := updateutil.FillFromLSBRelease(ctx, s.DUT(), s.RPCHint(), lsbContent); err != nil {
		s.Fatal("Failed to read from lsb file: ", err)
	}
	// Builder path is used in selecting the update image.
	builderPath := lsbContent[lsbrelease.BuilderPath]
	if builderPath == "" {
		s.Fatal("Builder path is missing")
	}

	// Create and set up a Nebraska client for an in-place update.
	nebraskaClient, nebraskaPort, err := updateutil.ConfigureNebraskaFromGS(ctx, rpcClient.Conn, s.DUT(), s.OutDir(), builderPath)
	if err != nil {
		s.Fatal("Failed to set up nebraska for an in-place update")
	}
	defer func(ctx context.Context) {
		if rpcClient != nil {
			// Recreating since the client can have a stale RPC connection after the reboot.
			nebraskaClient := nebraska.NewServiceClient(rpcClient.Conn)
			nebraskaClient.Stop(ctx, &empty.Empty{})
		}
	}(cleanupCtx)

	// Configure Nebraska to serve an enterprise rollback update if it is a rollback test.
	if _, err := nebraskaClient.SetIsRollback(ctx, &nebraska.SetIsRollbackRequest{IsRollback: param.isRollback}); err != nil {
		s.Fatal("Failed to remove rollback flag from Nebraska")
	}

	// Force an update check to update in-place.
	updateClient := autoupdate.NewUpdateServiceClient(rpcClient.Conn)
	if _, err := updateClient.CheckForUpdate(ctx, &autoupdate.UpdateRequest{
		OmahaUrl: fmt.Sprintf("http://127.0.0.1:%d/update", nebraskaPort),
	}); err != nil {
		s.Fatal("Failed to update in-place: ", err)
	}

	if err := updateutil.FakeFirmwareUpdate(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to fake firmware update: ", err)
	}
	defer func(ctx context.Context) {
		if err := updateutil.RevertFakeFirmwareUpdate(ctx, s.DUT()); err != nil {
			s.Error("Failed to reset fake firmware update: ", err)
		}
	}(cleanupCtx)

	// Disable updates via policy.
	pb.AddPolicies([]policy.Policy{
		&policy.DeviceAutoUpdateDisabled{Val: true},
	})
	pJSON, err = json.Marshal(pb)
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}

	if _, err := policyClient.UpdatePolicies(ctx, &policypkg.UpdatePoliciesRequest{
		PolicyJson: pJSON,
	}); err != nil {
		s.Fatal("Failed to update policy: ", err)
	}

	// Verify that the update is invalidated.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := updateutil.VerifyInvalidatedUpdate(ctx, s.DUT(), mainFWAct); err != nil {
			return err
		}

		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: time.Second}); err != nil {
		s.Fatal("Failed to verify the update invalidation: ", err)
	}

	// Reboot and try to ssh into a device to verify that the device still boots properly.
	if err := rpcClient.Close(ctx); err != nil {
		s.Error("Failed to close rpc connection: ", err)
	}
	rpcClient = nil

	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	// Recreating because the RPC connection is stale after the reboot.
	rpcClient, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	if err := updateutil.VerifyUpdateInvalidationPostReboot(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to verify that a device is in a valid state after reboot: ", err)
	}
}
