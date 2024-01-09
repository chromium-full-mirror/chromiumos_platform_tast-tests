// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcvpn"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCToHostVPN,
		Desc:         "Switch from an ARC VPN to a host VPN",
		Contacts:     []string{"cros-networking@google.com", "cassiewang@google.com"},
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline"},
		Fixture:      "arcBooted",
		SoftwareDeps: []string{"arc"},
	})
}

func ARCToHostVPN(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

	// Set up host VPN.
	conn, err := arcvpn.SetUpHostVPN(ctx, vpn.TypeL2TPIPsec)
	if err != nil {
		s.Fatal("Failed to setup host VPN: ", err)
	}
	defer func() {
		if err := conn.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up host VPN: ", err)
		}
	}()
	defer func() {
		if err := arcvpn.ForceStopARCVPN(cleanupCtx, a); err != nil {
			s.Error("Failed to clean up ARC VPN: ", err)
		}
	}()

	// Install and start the test app.
	testing.ContextLog(ctx, "Installing ArcVpnTest.apk")
	if err := a.Install(ctx, arc.APKPath(arcvpn.VPNTestAppAPK)); err != nil {
		s.Fatal("Failed to install app: ", err)
	}
	defer func() {
		testing.ContextLog(cleanupCtx, "Uninstalling ArcVpnTest.apk")
		if err := a.Uninstall(cleanupCtx, arcvpn.VPNTestAppPkg); err != nil {
			s.Fatal("Failed to uninstall ArcVpnTest.apk: ", err)
		}
	}()

	testing.ContextLog(ctx, "Preauthorizing ArcVpnTest")
	if _, err := a.Command(ctx, "dumpsys", "wifi", "authorize-vpn", arcvpn.VPNTestAppPkg).Output(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to execute authorize-vpn command: ", err)
	}

	testing.ContextLog(ctx, "Starting ArcVpnTest app")
	if err := arcvpn.StartARCVPN(ctx, a); err != nil {
		s.Fatal("Failed to send ArcVpnTest app: ", err)
	}

	// Make sure our test app is connected.
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.VPNTestAppPkg, arcvpn.VPNTestAppSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.VPNTestAppSvc, err)
	}

	// Host traffic gets routed to the ARC VPN correctly.
	if err := ping.ExpectPingSuccessWithTimeout(ctx, arcvpn.TunIP, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s from host: %v", arcvpn.TunIP, err)
	}

	// Connect to host VPN. Poll since there's a race between the ARC VPN disconnecting and the
	// host VPN connecting. It's possible that while the ARC VPN is still connected, the host
	// VPN tries to connect and fails since it's unintentionally trying to go through the ARC VPN.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := conn.Connect(ctx); err != nil {
			return errors.Wrap(err, "still not connected to VPN server")
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		s.Fatal("Failed to connect to VPN server: ", err)
	}

	// Make sure our test app is disconnected.
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.VPNTestAppPkg, arcvpn.VPNTestAppSvc, false); err != nil {
		s.Fatalf("Failed to stop %s: %v", arcvpn.VPNTestAppSvc, err)
	}

	// Facade ARC VPN is connected.
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.FacadeVPNPkg, arcvpn.FacadeVPNSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.FacadeVPNSvc, err)
	}

	// Host and ARC traffic are routed correctly.
	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping from host %s: %v", conn.Server.OverlayIPv4, err)
	}
	if err := arc.ExpectPingSuccess(ctx, a, "vpn", conn.Server.OverlayIPv4); err != nil {
		s.Fatalf("Failed to ping %s from ARC over 'vpn': %v", conn.Server.OverlayIPv4, err)
	}
}
