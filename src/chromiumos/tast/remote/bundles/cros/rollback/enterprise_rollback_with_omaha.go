// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"chromiumos/tast/remote/policyutil"
	aupb "chromiumos/tast/services/cros/autoupdate"
	pspb "chromiumos/tast/services/cros/policy"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnterpriseRollbackWithOmaha,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Example test for the enterprise rollback update",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"gabormagda@google.com", // Test author
		},
		BugComponent: "b:1031231",
		Attr:         []string{}, // Manual execution only.
		VarDeps:      []string{"rollback.EnterpriseRollbackWithOmaha.confirm", "rollback.EnterpriseRollbackWithOmaha.sourceVersion", "rollback.EnterpriseRollbackWithOmaha.targetVersion"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.autoupdate.UpdateService"},
		Timeout:      5 * time.Minute,
	})
}

// EnterpriseRollbackWithOmaha test must be provided the source and target image versions.
// The source version should be a full version string. The target can be
// just a prefix. Furthermore, test should be started with
//
//	-var=rollback.EnterpriseRollbackWithOmaha.confirm=ICanRollbackMyDUT
//
// to avoid accidental execution of the test.
//
// For example, to run a rollback from M96 to M94:
// tast run
//
//	-var=rollback.EnterpriseRollbackWithOmaha.confirm=ICanRollbackMyDUT
//	-var=rollback.EnterpriseRollbackWithOmaha.sourceVersion=14244.0.0
//	-var=rollback.EnterpriseRollbackWithOmaha.targetVersion=14092.
//	<ip> rollback.EnterpriseRollbackWithOmaha
func EnterpriseRollbackWithOmaha(ctx context.Context, s *testing.State) {
	if s.RequiredVar("rollback.EnterpriseRollbackWithOmaha.confirm") != "ICanRollbackMyDUT" {
		s.Log("You should only run this example test if you have manual access to your DUT")
		s.Log("After the update, you can restore the previous partition with the following command:")
		s.Log("\tupdate_engine_client --rollback --nopowerwash")

		s.Fatal("Failed to make sure it is an intentional test execution")
	}

	successfulUpdate := false

	func(ctx context.Context) {
		defer func(ctx context.Context) {
			if !successfulUpdate {
				if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
					s.Error("Failed to reset TPM after test: ", err)
				}
			}
		}(ctx)

		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
		defer cancel()

		// Reset TPM.
		if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
			s.Fatal("Failed to reset TPM: ", err)
		}

		// Connect to DUT.
		cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(cleanupCtx)

		// Create clients.
		policyClient := pspb.NewPolicyServiceClient(cl.Conn)
		updateClient := aupb.NewUpdateServiceClient(cl.Conn)

		// Create an empty /mnt/stateful_partition/etc/lsb-release if it doesn't
		// exist yet.
		if err := s.DUT().Conn().CommandContext(ctx, "touch", "/mnt/stateful_partition/etc/lsb-release").Run(); err != nil {
			s.Error("Failed to touch stateful lsb-release: ", err)
		}

		// Enable the DUT to receive updates.
		originalContent, err := signBoardName(ctx, updateClient)
		if err != nil {
			s.Fatal("Failed to enable the DUT to receive updates: ", err)
		}
		defer func(ctx context.Context, lsbContent []byte) {
			if _, err := updateClient.OverwriteStatefulLSBRelease(ctx, &aupb.LSBRelease{ContentJson: lsbContent}); err != nil {
				s.Log("Failed to restore lsb-release in the stateful partition: ", err)
			}
		}(cleanupCtx, originalContent)

		// Enroll DUT.
		pJSON, err := json.Marshal(policy.NewBlob())
		if err != nil {
			s.Fatal("Failed to serialize policies: ", err)
		}

		if _, err := policyClient.EnrollUsingChrome(ctx, &pspb.EnrollUsingChromeRequest{
			PolicyJson: pJSON,
		}); err != nil {
			s.Fatal("Failed to enroll using chrome: ", err)
		}
		defer policyClient.StopChromeAndFakeDMS(ctx, &empty.Empty{})

		targetVersion := s.RequiredVar("rollback.EnterpriseRollbackWithOmaha.targetVersion")

		// Set update policies.
		rollbackPolicies := []policy.Policy{
			// Note: the update will fail if the other partition already has the same image
			// that is selected below to rollback to.
			&policy.DeviceTargetVersionPrefix{Val: targetVersion}, // Pass by argument, e.g. "13982." for M92.
			&policy.DeviceRollbackAllowedMilestones{Val: 4},
			&policy.DeviceRollbackToTargetVersion{Val: 3}, // Roll back and stay on target version if OS version is newer than target. Try to carry over device-level configuration.
			&policy.ChromeOsReleaseChannel{Val: "stable-channel"},
			&policy.ChromeOsReleaseChannelDelegated{Val: false},
		}
		policyBlob := policy.NewBlob()
		policyBlob.AddPolicies(rollbackPolicies)

		pJSON, err = json.Marshal(policyBlob)
		if err != nil {
			s.Fatal("Failed to serialize policies: ", err)
		}
		if _, err := policyClient.UpdatePolicies(ctx, &pspb.UpdatePoliciesRequest{
			PolicyJson: pJSON,
		}); err != nil {
			s.Fatal("Failed to enroll using chrome: ", err)
		}

		// Get the update log files even if the update fails.
		defer func(ctx context.Context) {
			if err := linuxssh.GetFile(ctx, s.DUT().Conn(), "/var/log/update_engine.log", filepath.Join(s.OutDir(), "update_engine.log"), linuxssh.DereferenceSymlinks); err != nil {
				s.Log("Failed to save update engine log: ", err)
			}
		}(cleanupCtx)

		sourceVersion := s.RequiredVar("rollback.EnterpriseRollbackWithOmaha.sourceVersion")

		// Update DUT with an update from the official prod server.
		// The server is given explicitly because self-built images may not have
		// it configured in their lsb-release file.
		if _, err := updateClient.CheckForUpdate(ctx, &aupb.UpdateRequest{
			OmahaUrl:   "https://tools.google.com/service/update2",
			AppVersion: sourceVersion,
		}); err != nil {
			s.Fatal("Failed to check for updates: ", err)
		}

		successfulUpdate = true
	}(ctx)

	// Reboot the DUT.
	if successfulUpdate {
		s.Log("Update was successful, rebooting DUT")
		s.Log("Note: The DUT will remain enrolled after reboot")
		s.Log("Note: After the reboot the SSH connecton to the DUT is disabled,")
		s.Log("      manual restoration is required: update_engine_client --rollback --nopowerwash")

		rebootCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		// Restart in an independent process, so the SSH connection can be closed before the restart.
		s.DUT().Conn().CommandContext(rebootCtx, "nohup", "bash", "-c", "sleep 15; reboot;").Run() // Ignore the error.
	}
}

// signBoardName adds an entry to /mnt/stateful_partition/etc/lsb-release with a signed board name
// to enable the DUT to receive updates.
// Returns with the original content of /mnt/stateful_partition/etc/lsb-release so it can be restored after the update.
func signBoardName(ctx context.Context, client aupb.UpdateServiceClient) ([]byte, error) {
	// Get board name from /etc/lsb-release.
	response, err := client.LSBReleaseContent(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to read lsb-release")
	}

	var lsb map[string]string
	if err := json.Unmarshal(response.ContentJson, &lsb); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal lsb-relese content")
	}

	board, ok := lsb[lsbrelease.Board]
	if !ok {
		return nil, errors.New("failed to determine DUT board")
	}
	signedBoardName := board + "-signed-mp-v3keys"

	// Get content of /mnt/stateful_partition/etc/lsb-release.
	response, err = client.StatefulLSBReleaseContent(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to read lsb-release on the stateful partition")
	}

	testing.ContextLogf(ctx, "Adding the %q board name to lsb-release in the stateful partition", signedBoardName)
	var statefulLsb map[string]string
	if err := json.Unmarshal(response.ContentJson, &statefulLsb); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal stateful lsb-relese content")
	}
	statefulLsb[lsbrelease.Board] = signedBoardName

	newStatefulLsbJSON, err := json.Marshal(statefulLsb)
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize stateful lsb-release content")
	}

	if _, err := client.OverwriteStatefulLSBRelease(ctx, &aupb.LSBRelease{ContentJson: newStatefulLsbJSON}); err != nil {
		return nil, errors.Wrap(err, "failed to overwrite lsb-release in the stateful partition")
	}

	return response.ContentJson, nil
}
