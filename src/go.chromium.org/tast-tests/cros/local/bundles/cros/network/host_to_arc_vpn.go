// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcvpn"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HostToARCVPN,
		Desc:         "Switch from a host VPN to an ARC VPN",
		Contacts:     []string{"cros-networking@google.com", "jiejiang@google.com"},
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline"},
		Fixture:      "arcBooted",
		SoftwareDeps: []string{"arc"},
		HardwareDeps: arc.ArcAppHwDeps,
	})
}

func HostToARCVPN(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

	// Save the ARC network dumpsys as close to the time of error as possible, in case
	// further cleanup affects the network state.
	handler := arcnet.CreateNetworkDumpsysErrorHandler(cleanupCtx, a)
	s.AttachErrorHandlers(handler, handler)

	// Set up and connect to host VPN.
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

	if err := conn.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to VPN server: ", err)
	}
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.FacadeVPNPkg, arcvpn.FacadeVPNSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.FacadeVPNSvc, err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping from host %s: %v", conn.Server.OverlayIPv4, err)
	}
	if err := arcnet.ExpectPingSuccess(ctx, a, "vpn", conn.Server.OverlayIPv4); err != nil {
		s.Fatalf("Failed to ping %s from ARC over 'vpn': %v", conn.Server.OverlayIPv4, err)
	}

	// Install and start the test app.
	cleanupFunc, err := arcvpn.InstallAndPreAuthorizeARCVPN(ctx, a)
	if err != nil {
		s.Fatal("Failed to set up ARC VPN test app: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	testing.ContextLog(ctx, "Starting ArcVpnTest app")
	if err := arcvpn.StartARCVPN(ctx, a); err != nil {
		s.Fatal("Failed to send ArcVpnTest app: ", err)
	}

	// Make sure the host is disconnected.
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.FacadeVPNPkg, arcvpn.FacadeVPNSvc, false); err != nil {
		s.Fatalf("Failed to stop %s: %v", arcvpn.FacadeVPNSvc, err)
	}

	// Make sure our test app is connected.
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.VPNTestAppPkg, arcvpn.VPNTestAppSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.VPNTestAppSvc, err)
	}

	// Host traffic gets routed to the ARC VPN correctly.
	if err := ping.ExpectPingSuccessWithTimeout(ctx, arcvpn.TunIP, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s from host: %v", arcvpn.TunIP, err)
	}
}
