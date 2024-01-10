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
		Func:         ARCVPNCrash,
		Desc:         "When ARC VPN crashes, host VPN is still reachable in ARC",
		Contacts:     []string{"cros-networking@google.com", "cassiewang@google.com"},
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Fixture:      "shillResetWithArcBooted",
		Params: []testing.Param{{
			Name:              "container",
			Val:               "container",
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			Val:               "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func ARCVPNCrash(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

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

	// Check that if ArcHostVpnService is stopped unexpectedly (simulating some sort
	// of error), the host VPN is still reachable from within ARC.
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
		s.Fatalf("Failed to ping from ARC %s: %v", conn.Server.OverlayIPv4, err)
	}
	if err := arcvpn.ForceStopARCVPN(ctx, a); err != nil {
		s.Fatal("Failed to crash ArcHostVpnService: ", err)
	}
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.FacadeVPNPkg, arcvpn.FacadeVPNSvc, false); err != nil {
		s.Fatalf("Failed to stop %s: %v", arcvpn.FacadeVPNSvc, err)
	}
	// VPN should still be reachable even without ArcHostVpnService
	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping from host %s: %v", conn.Server.OverlayIPv4, err)
	}
	// arc0/eth0 are hardcoded in ARC as the iface of our fake ethernet network that VPN traffic
	// will fall back to if there's an issue with the ARC VPN.
	arcVersion := s.Param().(string)
	network := "eth0"
	if arcVersion == "container" {
		network = "arc0"
	}
	if err := arc.ExpectPingSuccess(ctx, a, network, conn.Server.OverlayIPv4); err != nil {
		s.Fatalf("Failed to ping %s from ARC over %q: %v", conn.Server.OverlayIPv4, network, err)
	}
}
