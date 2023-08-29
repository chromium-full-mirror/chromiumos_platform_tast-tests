// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floatingworkspace

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/services/cros/floatingworkspace"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SyncFloatingWorkspace,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check if we can create a template on one device and receive it on another one when desk template sync is enabled",
		Contacts: []string{
			"cros-commercial-productivity-eng@google.com",
			"zhumatthew@google.com",
		},
		// Chrome OS Server Projects > Enterprise Management > Commercial Productivity
		BugComponent: "b:1020793",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "android_vm", "no_kernel_upstream"},
		ServiceDeps:  []string{"tast.cros.floatingworkspace.TemplateSyncService"},
		Timeout:      420 * time.Second,
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Vars:         []string{"floatingworkspace.cros_username", "floatingworkspace.cros_password"},
	})
}

// SyncFloatingWorkspace tests that we can enable desk template sync on two DUTs in a single test.
func SyncFloatingWorkspace(ctx context.Context, s *testing.State) {
	d1 := s.DUT()
	client1, err := rpc.Dial(ctx, d1, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT")
	}
	defer client1.Close(ctx)

	fls1 := floatingworkspace.NewTemplateSyncServiceClient(client1.Conn)
	loginReq := &floatingworkspace.CrOSLoginRequest{}
	loginReq.Username = s.RequiredVar("floatingworkspace.cros_username")
	loginReq.Password = s.RequiredVar("floatingworkspace.cros_password")
	loginReq.SignInOption = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	if _, err = fls1.NewChromeLogin(ctx, loginReq); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer fls1.CloseChrome(ctx, &empty.Empty{})

	// Sign out on main DUT.
	if _, err = fls1.Signout(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to sign out: ", err)
	}

	// Connect to the companion device cd1.
	d2 := s.CompanionDUT("cd1")
	if d2 == nil {
		s.Fatal("Failed to get companion DUT cd1")
	}
	client2, err := rpc.Dial(ctx, d2, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the compaion device cd1")
	}
	defer client2.Close(ctx)
	fls2 := floatingworkspace.NewTemplateSyncServiceClient(client2.Conn)
	if _, err = fls2.NewChromeLogin(ctx, loginReq); err != nil {
		s.Fatal("Failed to start Chrome on companion device: ", err)
	}
	defer fls2.CloseChrome(ctx, &empty.Empty{})

	// Wait for floating workspace desk capture on companion device and sign out.
	if _, err = fls2.WaitForFloatingWorkspaceCapture(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to wait for floating workspace capture: ", err)
	}
	if _, err = fls2.Signout(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to sign out on companion device: ", err)
	}

	// Sign back in to main DUT.
	if _, err = fls1.SignBackIn(ctx, loginReq); err != nil {
		s.Fatal("Failed to sign back in: ", err)
	}

	// Verify floating workspace is synced to main DUT.
	if _, err = fls1.VerifyFloatingWorkspace(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to verify synced floating workspace on companion device: ", err)
	}

	if _, err = fls1.CloseChrome(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close chrome: ", err)
	}

}
