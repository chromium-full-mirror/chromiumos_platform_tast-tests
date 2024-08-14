// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package remotecommands

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/protobuf/proto"

	empb "go.chromium.org/chromiumos/policy/chromium/policy/enterprise_management_proto"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevicePowerwash,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks the behavior of sending device powerwash remote command",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mohamedaomar@google.com", // Test author
		},
		// ChromeOS > Software > Commercial (Enterprise) > Remote Management > Remote Commands
		BugComponent: "b:1206077",
		Attr:         []string{"group:powerwash-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.ChromeUIService",
		},
		Timeout: 30 * time.Minute,
	})
}

func DevicePowerwash(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time separately for logging and cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

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

	customPowerwash := func(ctx context.Context) error {
		req := &empb.RemoteCommand{
			Type: &[]empb.RemoteCommand_Type{empb.RemoteCommand_DEVICE_REMOTE_POWERWASH}[0],
		}
		b, err := proto.Marshal(req)
		if err != nil {
			return errors.Wrap(err, "failed to marshal the RemoteCommand")
		}
		if _, err := policyClient.SendRemoteCommand(ctx, &pspb.SendRemoteCommandRequest{
			RemoteCommand: b,
		}); err != nil {
			return errors.Wrap(err, "couldn't send the remote command request")
		}
		if _, err := policyClient.RefreshRemoteCommands(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to refresh remote commands")
		}
		if err := s.DUT().WaitUnreachable(ctx); err != nil {
			return errors.Wrap(err, "failed to wait till DUT is unreachable")
		}
		if err := s.DUT().WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to DUT after powerwash")
		}
		return nil
	}

	if err := policyutil.Powerwash(ctx, s.CloudStorage(), s.DUT(), s.PushedFilesToDUT(""), customPowerwash); err != nil {
		s.Fatal("Powerwash failed: ", err)
	}

	// Check if powerwash happened by checking if we're on the welcome screen.
	// TODO(b/359595368): create some file and check if it was removed due to powerwash.
	cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after powerwash: ", err)
	}
	defer cl.Close(cleanupCtx)

	crSvc := ui.NewChromeServiceClient(cl.Conn)
	defer crSvc.Close(cleanupCtx, &empty.Empty{})
	if _, err := crSvc.New(ctx, &ui.NewRequest{
		LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
		KeepState:                    true,
		SigninProfileTestExtensionId: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}); err != nil {
		s.Fatal("Failed to boot DUT to OOBE screen: ", err)
	}

	// Wait for WelcomeScreen to appear and OOBE to be ready.
	crUISvc := ui.NewChromeUIServiceClient(cl.Conn)
	if _, err := crUISvc.WaitForWelcomeScreen(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to wait for OOBE welcome screen: ", err)
	}
}
