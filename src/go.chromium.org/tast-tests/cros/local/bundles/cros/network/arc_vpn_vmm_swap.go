// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/swap"
	"go.chromium.org/tast-tests/cros/local/arc/vm"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcvpn"
	"go.chromium.org/tast-tests/cros/local/network/ping"
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
		// ChromeOS > Platform > Virtualization > VM Technology
		BugComponent: "b:930563",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"android_vm", "chrome", "crosvm_swap"},
		Fixture:      "arcBootedBoostedVmmSwap",
		Timeout:      5 * time.Minute,
	})
}

func ArcVpnVmmSwap(ctx context.Context, s *testing.State) {
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
	if err := a.Install(ctx, arc.APKPath(arcvpn.VPNTestAppAPK)); err != nil {
		s.Fatal("Failed to install app: ", err)
	}

	if _, err := a.Command(ctx, "dumpsys", "wifi", "authorize-vpn", arcvpn.VPNTestAppPkg).Output(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to execute authorize-vpn command: ", err)
	}

	// Only components with android.permission.BIND_VPN_SERVICE can bind to
	// VPNs, so we can't directly start the service from a non-root shell.
	// Instead, have a broadcast receiver ask that the system start the VPN.
	if _, err := a.BroadcastIntent(ctx, arcvpn.VPNTestAppBroadcast, "--include-stopped-packages", "--receiver-include-background"); err != nil {
		s.Fatal("Failed to send broadcast: ", err)
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
