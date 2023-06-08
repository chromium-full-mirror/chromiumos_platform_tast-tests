// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcvpn"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCVPNConnect,
		Desc:         "Host VPN is mirrored with ARC VPN properly",
		Contacts:     []string{"cros-networking@google.com", "cassiewang@google.com"},
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Fixture:      "shillResetWithArcBooted",
		SoftwareDeps: []string{"arc"},
	})
}

// ARCVPNConnect differs from VPNConnect in that ARCVPNConnect focuses on
// testing the VPN that's started in ARC to mirror the host VPN. See b/147256449
// for more details. Much of the testing around host VPN setup is left to
// VPNConnect to verify.
func ARCVPNConnect(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 3*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

	conn, err := arcvpn.SetUpHostVPN(ctx, vpn.TypeL2TPIPsec)
	if err != nil {
		s.Fatal("Failed to setup host VPN: ", err)
	}
	defer conn.Cleanup(cleanupCtx)

	// Verify ArcHostVpnService can connect and disconnect properly following the host VPN
	// lifecycle events.
	if err := conn.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to VPN server: ", err)
	}
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.FacadeVPNPkg, arcvpn.FacadeVPNSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.FacadeVPNSvc, err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping from host %s: %v", conn.Server.OverlayIPv4, err)
	}
	if err := arc.ExpectPingSuccess(ctx, a, "vpn", conn.Server.OverlayIPv4); err != nil {
		s.Fatalf("Failed to ping %s from ARC over 'vpn': %v", conn.Server.OverlayIPv4, err)
	}

	// Disconnect
	if err := conn.Disconnect(ctx); err != nil {
		s.Error("Failed to disconnect VPN: ", err)
	}
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.FacadeVPNPkg, arcvpn.FacadeVPNSvc, false); err != nil {
		s.Fatalf("Failed to stop %s: %v", arcvpn.FacadeVPNSvc, err)
	}
	if err := arc.ExpectPingSuccess(ctx, a, "vpn", conn.Server.OverlayIPv4); err == nil {
		s.Fatalf("Expected unable to ping %s from ARC over 'vpn', but was reachable", conn.Server.OverlayIPv4)
	}

	// Reconnect
	if err := waitForConnect(ctx, conn); err != nil {
		s.Fatal("Failed to reconnect to VPN: ", err)
	}
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.FacadeVPNPkg, arcvpn.FacadeVPNSvc, true); err != nil {
		s.Fatalf("Failed to start %s on reconnection: %v", arcvpn.FacadeVPNSvc, err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping from host %s: %v", conn.Server.OverlayIPv4, err)
	}
	if err := arc.ExpectPingSuccess(ctx, a, "vpn", conn.Server.OverlayIPv4); err != nil {
		s.Fatalf("Failed to ping %s from ARC over 'vpn': %v", conn.Server.OverlayIPv4, err)
	}
}

func waitForConnect(ctx context.Context, conn *vpn.Connection) error {
	// Reconnecting right after a disconnect takes some time for the reconnection to succeed.
	// Poll for a bit since it should be a transient issue.
	if err := testing.Poll(ctx, conn.Connect, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return err
	}
	return nil
}
