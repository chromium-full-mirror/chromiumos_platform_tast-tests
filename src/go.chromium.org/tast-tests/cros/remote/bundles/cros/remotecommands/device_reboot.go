// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package remotecommands

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/protobuf/proto"

	empb "chromiumos/policy/chromium/policy/enterprise_management_proto"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/remote/dut"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	rebootCheckTimeout = 3 * time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceReboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks the behavior of sending device reboot remote command",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mohamedaomar@google.com", // Test author
		},
		// ChromeOS > Software > Commercial (Enterprise) > Remote Management > Remote Commands
		BugComponent: "b:1206077",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
		},
		Timeout: rebootCheckTimeout,
	})
}

func DeviceReboot(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time separately for logging and cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	// Retrieve a current boot ID to check if the reboot happens.
	bootID, err := dut.ReadBootID(ctx, s.DUT().Conn())
	if err != nil {
		s.Fatal("Failed to read current boot ID: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	pJSON, err := json.Marshal(policy.NewBlob())
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}
	// Start the fakedms.
	if _, err := policyClient.EnrollUsingChrome(ctx, &pspb.EnrollUsingChromeRequest{
		PolicyJson: pJSON,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}

	req := &empb.RemoteCommand{
		Type: &[]empb.RemoteCommand_Type{empb.RemoteCommand_DEVICE_REBOOT}[0],
	}
	b, err := proto.Marshal(req)
	if err != nil {
		s.Fatal("Failed to marshal the RemoteCommand: ", err)
	}
	if _, err := policyClient.SendRemoteCommand(ctx, &pspb.SendRemoteCommandRequest{
		RemoteCommand: b,
	}); err != nil {
		s.Fatal("Couldn't send the remote command request: ", err)
	}

	if _, err := policyClient.RefreshRemoteCommands(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to refresh remote commands: ", err)
	}

	// User session has 5 min delay, so clicking on restart now is necessary.
	if _, err := policyClient.FindAndClickRestartNowButton(ctx, &empty.Empty{}); err != nil {
		s.Error("Failed to find and click on the restart now button: ", err)
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

		if id == bootID {
			return errors.New("boot_id is supposed to change")
		}

		return nil
	}, &testing.PollOptions{Timeout: rebootCheckTimeout, Interval: time.Second}); err != nil {
		s.Fatal("Failed to wait for reboot: ", err)
	}
}
