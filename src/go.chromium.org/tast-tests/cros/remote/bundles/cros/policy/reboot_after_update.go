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

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	preUpdateTimeout   = 1 * time.Minute
	rebootCheckTimeout = 1 * time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RebootAfterUpdate,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies RebootAfterUpdate policy by setting the policy, updating in-place and checking if the device reboots",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"artyomchen@google.com", // Test author
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
		},
		ServiceDeps: []string{
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
		},
		Timeout: preUpdateTimeout + updateutil.UpdateTimeout + rebootCheckTimeout,
	})
}

func RebootAfterUpdate(ctx context.Context, s *testing.State) {
	for _, param := range []struct {
		name string                    // Subtest name.
		ps   *policy.RebootAfterUpdate // Policy value.
	}{
		{
			name: "reboot_after_update_on",
			ps:   &policy.RebootAfterUpdate{Val: true},
		},
		{
			name: "reboot_after_update_off",
			ps:   &policy.RebootAfterUpdate{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
				s.Fatal("Failed to reset TPM: ", err)
			}
			// Ensure that the system state is reset after each run.
			defer func() {
				if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
					s.Error("Failed to reset TPM: ", err)
				}
			}()

			cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
			if err != nil {
				s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
			}
			defer cl.Close(ctx)

			// Enroll the DUT and set the policy.
			pb := policy.NewBlob()
			// Explicitly configure the UptimeLimit policy so that
			// it doesn't conflict with the RebootAfterUpdate policy.
			pb.AddPolicies([]policy.Policy{
				&policy.UptimeLimit{Val: 0},
				param.ps,
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
			defer policyClient.StopChromeAndFakeDMS(ctx, &empty.Empty{})

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

			// Retrieve a current boot ID to check if the reboot happens.
			bootID, err := dut.ReadBootID(ctx, s.DUT().Conn())
			if err != nil {
				s.Fatal("Failed to read current boot ID: ", err)
			}

			// Update the DUT.
			// Ignore errors for the reboot_after_update_on subtest,
			// since the method throws cleanup errors due to the reboot.
			// For testing purposes, it is enough to check that the reboot
			// happened when the policy is enabled.
			if err := updateutil.UpdateFromGS(ctx, s.DUT(), s.OutDir(), s.RPCHint(), builderPath); !param.ps.Val && err != nil {
				s.Fatal("Failed to update image: ", err)
			}

			// Wait for the reboot and the boot_id change.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
				if err := s.DUT().WaitConnect(ctx); err != nil {
					return errors.Wrap(err, "failed to connect to DUT")
				}

				id, err := dut.ReadBootID(ctx, s.DUT().Conn())
				if err != nil {
					return errors.Wrap(err, "failed to read boot_id")
				}

				if param.ps.Val && id == bootID {
					return errors.New("boot_id is supposed to change")
				} else if !param.ps.Val && id != bootID {
					return errors.New("boot_id is not supposed to change")
				}

				return nil
			}, &testing.PollOptions{Timeout: rebootCheckTimeout, Interval: time.Second}); err != nil {
				s.Fatal("Failed to wait for reboot: ", err)
			}

			// Extra reboot is needed for unknown reasons.
			// Tast cannot execute SSH commands and clean up the state
			// after an update has been applied, unless the device is rebooted.
			if !param.ps.Val {
				if err := s.DUT().Reboot(ctx); err != nil {
					s.Fatal("Failed to reboot: ", err)
				}
			}
		})
	}
}
