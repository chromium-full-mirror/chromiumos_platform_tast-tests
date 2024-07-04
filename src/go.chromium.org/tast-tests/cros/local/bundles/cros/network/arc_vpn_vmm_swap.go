// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/swap"
	"go.chromium.org/tast-tests/cros/local/arc/vm"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcvpn"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ArcVpnVmmSwap,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that vmm-swap doesn't interfere with Android VPNs",
		Contacts: []string{
			"cros-vm-technology@google.com",
			"kawasin@google.com",
			"hikalium@chromium.org",
		},
		// ChromeOS > Platform > baseOS > Virtualization > ARC++ & ARCVM
		BugComponent: "b:882467",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"android_vm", "chrome", "crosvm_swap"},
		Fixture:      "arcBootedBoostedVmmSwap",
		Timeout:      5 * time.Minute,
	})
}

func ArcVpnVmmSwap(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	s.Log("Launching swap test activity to disable swap")
	if err := a.Install(ctx, arc.APKPath(swap.TestApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	swapActivity, err := arc.NewActivity(a, swap.Pkg, swap.Activity)
	if err != nil {
		s.Fatal("Failed to create vpn launcher activity: ", err)
	}

	err = swapActivity.Start(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch swap test activity: ", err)
	}

	s.Log("Waiting for swap to be disabled")
	socketPath, err := vm.SocketPath()
	if err != nil {
		s.Fatal("Failed to get crosvm sock: ", err)
	}

	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Ready}); err != nil {
		s.Fatal("Failed to wait for swap to be disabled: ", err)
	}

	s.Log("Starting VPN")
	cleanupFunc, err := arcvpn.InstallAndPreAuthorizeARCVPN(ctx, a)
	if err != nil {
		s.Fatal("Failed to set up ARC VPN test app")
	}
	defer cleanupFunc(cleanupCtx)

	if err := arcvpn.StartARCVPN(ctx, a); err != nil {
		s.Fatal("Failed to send ArcVpnTest app: ", err)
	}

	s.Log("Validating VPN")
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.VPNTestAppPkg, arcvpn.VPNTestAppSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.VPNTestAppSvc, err)
	}

	if err := ping.ExpectPingSuccessWithTimeout(ctx, arcvpn.TunIP, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s from host: %v", arcvpn.TunIP, err)
	}

	s.Log("Stopping activity and waiting for swap to be enabled")
	err = swapActivity.Stop(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to stop swap test activity: ", err)
	}

	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Pending}); err != nil {
		s.Fatal("Failed to wait for swap to be enabled: ", err)
	}

	s.Log("Revalidating VPN after swap")
	if err := ping.ExpectPingSuccessWithTimeout(ctx, arcvpn.TunIP, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s from host: %v", arcvpn.TunIP, err)
	}
}
