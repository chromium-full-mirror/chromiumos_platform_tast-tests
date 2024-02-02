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
		Func:         ARCVPNDisabled,
		Desc:         "ARC VPN doesn't start when flag is off",
		Contacts:     []string{"cros-networking@google.com", "cassiewang@google.com"},
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "vpnEnvWithArcBooted",
		SoftwareDeps: []string{"arc"},
	})
}

func ARCVPNDisabled(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
	defer cancel()

	a := s.FixtValue().(vpn.FixtureEnv).ARC

	if err := arcvpn.SetARCVPNEnabled(ctx, a, false); err != nil {
		s.Fatal("Failed to disable ARC VPN: ", err)
	}
	defer func() {
		if err := arcvpn.SetARCVPNEnabled(ctx, a, true); err != nil {
			s.Fatal("Failed to enable ARC VPN: ", err)
		}
	}()

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
	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping from host %s: %v", conn.Server.OverlayIPv4, err)
	}
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.FacadeVPNPkg, arcvpn.FacadeVPNSvc, false); err != nil {
		s.Fatalf("Failed to stop %s: %v", arcvpn.FacadeVPNSvc, err)
	}
	if err := arc.ExpectPingSuccess(ctx, a, "vpn", conn.Server.OverlayIPv4); err == nil {
		s.Fatalf("Expected unable to ping %s from ARC over 'vpn', but was reachable", conn.Server.OverlayIPv4)
	}
}
