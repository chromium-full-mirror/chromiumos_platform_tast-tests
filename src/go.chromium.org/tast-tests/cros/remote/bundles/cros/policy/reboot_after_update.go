// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/remote/dut"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/updateutil"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	preUpdateTimeout   = 1 * time.Minute
	rebootCheckTimeout = 1 * time.Minute
	cleanupTimeout     = 1 * time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RebootAfterUpdate,
		Desc: "Verifies RebootAfterUpdate policy by setting the policy, updating in-place and checking if the device reboots",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test owner
		},
		Fixture:      fixture.Autoupdate, // Cleans the updates and ensures the original image is restored.
		BugComponent: "b:1031231",        // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Version Control
		Attr:         []string{"group:autoupdate"},
		SoftwareDeps: []string{"reboot", "chrome", "auto_update_stable"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UptimeLimit{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.RebootAfterUpdate{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// As an IT leader/manager, I want to control the details of
				// how my managed devices update in order to minimize
				// disruption to my business operations.
				// So I define if my managed kiosk devices or
				// managed user devices without an active session should
				// automatically reboot after downloading an update without
				// the need to manually reboot them.
				// reboot=yes
				// COM_FOUND_CUJ12_TASK3_WF1
				Value: "screenplay-30ab5088-d395-4c50-a424-6307cde95c51",
			},
			{
				Key: "feature_id",
				// As an IT leader/manager, I want to control the details of
				// how my managed devices update in order to minimize
				// disruption to my business operations.
				// So I define if my managed kiosk devices or
				// managed user devices without an active session should
				// automatically reboot after downloading an update without
				// the need to manually reboot them.
				// reboot=no
				// COM_FOUND_CUJ12_TASK3_WF2
				Value: "screenplay-6c00ff64-eb5d-4176-89ab-a260ad9ea279",
			},
			{
				Key: "feature_id",
				// 1. Configure an OU with a device policy A set to X
				// 2. Enroll a device
				// 3. Ensure that the policy A is set to X on the device
				// COM_FOUND_CUJ31_TASK2_WF1
				Value: "screenplay-a66aea3f-5e72-4644-a309-412d58592045",
			},
		},
		ServiceDeps: []string{
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
		},
		Params: []testing.Param{{
			Name: "reboot_after_update_on",
			Val:  &policy.RebootAfterUpdate{Val: true},
		}, {
			Name: "reboot_after_update_off",
			Val:  &policy.RebootAfterUpdate{Val: false},
		}},
		Timeout: preUpdateTimeout + updateutil.UpdateTimeout + rebootCheckTimeout + cleanupTimeout,
	})
}

func RebootAfterUpdate(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTimeout)
	defer cancel()

	if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}
	// Ensure that the system state is reset after each run.
	defer func() {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(cleanupCtx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM: ", err)
		}
	}()

	// Local setup is in a function to close all connections in deferred calls before update and reboot.
	func() {
		cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(cleanupCtx)

		// Enroll the DUT and set the policy.
		pb := policy.NewBlob()
		// Explicitly configure the UptimeLimit policy so that
		// it doesn't conflict with the RebootAfterUpdate policy.
		pb.AddPolicies([]policy.Policy{
			&policy.UptimeLimit{Val: 0},
			s.Param().(*policy.RebootAfterUpdate),
		})
		pJSON, err := json.Marshal(pb)
		if err != nil {
			s.Fatal("Failed to serialize policies: ", err)
		}
		policyClient := pspb.NewPolicyServiceClient(cl.Conn)
		if _, err := policyClient.EnrollUsingChrome(ctx, &pspb.EnrollUsingChromeRequest{
			PolicyJson: pJSON,
			ExtraArgs:  "--min-reboot-uptime-ms=1000",
			SkipLogin:  true,
		}); err != nil {
			s.Fatal("Failed to enroll: ", err)
		}
		defer policyClient.StopChromeAndFakeDMS(cleanupCtx, &empty.Empty{})
	}()

	// Read the current image version from the lsb file
	// for the in-place update.
	lsbContent := map[string]string{
		lsbrelease.Version:     "",
		lsbrelease.BuilderPath: "",
	}
	if err := updateutil.FillFromLSBRelease(ctx, s.DUT(), s.RPCHint(), lsbContent); err != nil {
		s.Fatal("Failed to read from lsb file: ", err)
	}
	// Builder path is used in selecting the update image.
	builderPath := lsbContent[lsbrelease.BuilderPath]

	// Retrieve the current boot ID to check if the reboot happens.
	oldBootID, err := dut.ReadBootID(ctx, s.DUT().Conn())
	if err != nil {
		s.Fatal("Failed to read current boot ID: ", err)
	}

	// Retrieve the current root partition name to check if the update happens.
	oldRootPartition, err := dut.ReadCurrentRootPartitionName(ctx, s.DUT().Conn())
	if err != nil {
		s.Fatal("Failed to get current root partition: ", err)
	}

	// Ignore update errors for the reboot_after_update_on subtest,
	// since the method throws cleanup errors due to the automatic reboot.
	err = updateutil.UpdateFromGS(ctx, s.DUT(), s.OutDir(), s.RPCHint(), builderPath)

	automaticReboot := s.Param().(*policy.RebootAfterUpdate).Val
	if !automaticReboot {
		if err != nil {
			s.Fatal("Failed to update image: ", err)
		}

		// Check that the boot ID is still the same
		currentBootID, err := dut.ReadBootID(ctx, s.DUT().Conn())
		if err != nil {
			s.Fatal("Failed to read current boot ID: ", err)
		}
		if oldBootID != currentBootID {
			s.Fatal("boot_id is not supposed to change")
		}

		// Check that the partition name is still the same.
		currnetRootPartition, err := dut.ReadCurrentRootPartitionName(ctx, s.DUT().Conn())
		if err != nil {
			s.Fatal("Failed to get current root partition: ", err)
		}
		if oldRootPartition != currnetRootPartition {
			s.Fatal("Partition name is not supposed to change")
		}

		// If reboot is not automatic we need to reboot manually to be able to clean up.
		if err := s.DUT().Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
	} else {
		// Await the reboot and ensure root partition changed.
		if err := updateutil.WaitForUpdateReboot(ctx, s.DUT(), oldRootPartition); err != nil {
			s.Fatal("Failed to await reboot into the new image: ", err)
		}
	}
}
