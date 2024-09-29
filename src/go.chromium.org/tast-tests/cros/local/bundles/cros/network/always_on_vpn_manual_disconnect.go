// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     AlwaysOnVPNManualDisconnect,
		Desc:     "An always-on VPN should be reconnected soon after disconnected by user",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		Attr:     []string{"group:mainline", "informational"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Fixture:      "vpnEnvWithCerts",
		Timeout:      1 * time.Minute,
		Params: []testing.Param{{
			Name: "openvpn",
			Val:  vpn.TypeOpenVPN,
		}, {
			Name: "l2tp_ipsec",
			Val:  vpn.TypeL2TPIPsec,
		},
		},
	})
}

// AlwaysOnVPNManualDisconnect sets up an always-on VPN connection and
// disconnects it manually, verifies that the VPN service should be reconnected
// soon after a brief time (¬3 seconds).
func AlwaysOnVPNManualDisconnect(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewDisablePortalDetectionHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	s.Log("Setting up VPN connection")
	vpnType := s.Param().(vpn.Type)
	opts := []vpn.Option{
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
	}
	vpnConn, err := vpn.StartConnection(ctx, nil /*env*/, vpnType, opts...)
	if err != nil {
		s.Fatal("Failed to set up VPN connection: ", err)
	}
	defer func() {
		if err := vpnConn.Cleanup(cleanupCtx); err != nil {
			s.Log("Failed to clean up VPN connection")
		}
	}()

	svc := vpnConn.Service()

	// Change the Always-on VPN mode.
	const vpnMode = shillconst.AlwaysOnVPNModeBestEffort
	s.Logf("Setting always-on-vpn mode in shill to %s", vpnMode)
	resetFunc, err := vpn.SetAlwaysOnVPN(ctx, vpnMode, svc)
	if err != nil {
		s.Fatal("Failed to configure Always-on VPN: ", err)
	}
	defer resetFunc(cleanupCtx)

	// The VPN should be connected after StartConnection(). Check it here before
	// proceeding the test.
	if isConnected, err := svc.IsConnected(ctx); err != nil {
		s.Fatal("Failed to get VPN connected state: ", err)
	} else if !isConnected {
		s.Fatal("VPN service is not connected after setting always-on-vpn mode, want connected")
	}

	// Use PropertiesWatcher to catch the PropertyChanged signal from shill. Since
	// the reconnection can happen very soon, checking the state after
	// disconnection may not see the transition to the idle state.
	pw, err := svc.CreateWatcher(ctx)
	if err != nil {
		s.Fatal("Failed to create properties watcher for the VPN service: ", err)
	}
	defer func() {
		if err := pw.Close(cleanupCtx); err != nil {
			s.Log("Failed to close properties watcher for the VPN service: ", err)
		}
	}()

	s.Log("Disconnecting VPN service to verify service can be connected automatically")
	if err := svc.Disconnect(ctx); err != nil {
		s.Fatal("Failed to disconnect VPN service: ", err)
	}

	s.Log("Waiting for VPN service idle after disconnect")
	idleCtx, idleCancel := context.WithTimeout(ctx, 3*time.Second)
	defer idleCancel()
	if err := pw.Expect(idleCtx, shillconst.ServicePropertyState, shillconst.ServiceStateIdle); err != nil {
		s.Fatal("Failed to wait for VPN service state to become idle")
	}

	s.Log("Waiting for VPN service online")
	onlineCtx, onlineCancel := context.WithTimeout(ctx, 10*time.Second)
	defer onlineCancel()
	if err := pw.Expect(onlineCtx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline); err != nil {
		s.Fatal("Failed to wait for VPN service state to become online")
	}
}
