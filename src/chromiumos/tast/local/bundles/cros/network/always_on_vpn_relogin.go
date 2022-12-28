// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/common/pkcs11/netcertstore"
	"chromiumos/tast/common/shillconst"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/network/vpn"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/network/routing"
	"chromiumos/tast/local/shill"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

// alwaysOnVPNReloginTestCase defines mode and config of the VPN
// we want to set up in the test
type alwaysOnVPNReloginTestCase struct {
	// mode of always on VPN we want to test.
	mode string
	// configurarion of host VPN.
	config vpn.Config
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AlwaysOnVPNRelogin,
		Desc:         "Host VPN client can be configured as always-on VPN and connected automatically after logout and login",
		Contacts:     []string{"cros-networking@google.com", "chuweih@google.com"},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:237712133",
		Fixture:      "vpnEnvWithCerts",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "strict_mode_l2tp_ipsec",
				Val: alwaysOnVPNReloginTestCase{
					mode:   shillconst.AlwaysOnVPNModeStrict,
					config: *vpn.NewConfig(vpn.TypeL2TPIPsec),
				},
			},
			{
				Name: "best_effort_mode_l2tp_ipsec",
				Val: alwaysOnVPNReloginTestCase{
					mode:   shillconst.AlwaysOnVPNModeBestEffort,
					config: *vpn.NewConfig(vpn.TypeL2TPIPsec),
				},
			},
			{
				Name: "strict_mode_openvpn",
				Val: alwaysOnVPNReloginTestCase{
					mode:   shillconst.AlwaysOnVPNModeStrict,
					config: *vpn.NewConfig(vpn.TypeOpenVPN),
				},
			},
			{
				Name: "best_effort_mode_openvpn",
				Val: alwaysOnVPNReloginTestCase{
					mode:   shillconst.AlwaysOnVPNModeBestEffort,
					config: *vpn.NewConfig(vpn.TypeOpenVPN),
				},
			},
		},
		Timeout: 10 * time.Minute,
	})
}

// AlwaysOnVPNRelogin tests always on VPN can be configured and auto reconnect
// after user logout and login.
func AlwaysOnVPNRelogin(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}
	// Set up virtualnet environment.
	testEnv := routing.NewTestEnvWithoutResetProfile()
	if err := testEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to set up routing test env: ", err)
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down routing test env: ", err)
		}
	}(cleanupCtx)

	cred := chrome.Creds{User: netcertstore.TestUsername, Pass: netcertstore.TestPassword}
	cr, err := chrome.New(
		ctx,
		chrome.KeepState(),     // to avoid resetting TPM
		chrome.FakeLogin(cred), // to use the same user as certs are installed for
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	// Disable portal detection to avoid default network fallback.
	if err := m.SetProperty(ctx, shillconst.ProfilePropertyCheckPortalList, "wifi,cellular"); err != nil {
		s.Fatal("Failed to disable portal detection on ethernet: ", err)
	}
	defer func() {
		if err := m.SetProperty(cleanupCtx, shillconst.ProfilePropertyCheckPortalList, "ethernet,wifi,cellular"); err != nil {
			s.Fatal("Failed to restore portal detection on ethernet: ", err)
		}
	}()

	// Set up new VPN connection based on the config.
	config := s.Param().(alwaysOnVPNReloginTestCase).config
	config.CertVals = s.FixtValue().(vpn.FixtureEnv).CertVals

	conn, err := vpn.NewConnectionWithEnvs(ctx, config, testEnv.BaseServer, nil)
	if err != nil {
		s.Fatal("Failed to create connection object: ", err)
	}

	if err := conn.SetUp(ctx); err != nil {
		s.Fatal("Failed to setup VPN server: ", err)
	}

	// Use set up host VPN as service and change the Always-on VPN mode.
	profile, err := m.ActiveProfile(ctx)
	if err != nil {
		s.Fatal("Failed to get active profile: ", err)
	}
	vpnMode := s.Param().(alwaysOnVPNReloginTestCase).mode
	if err := profile.SetAlwaysOnVPN(ctx, vpnMode, conn.Service()); err != nil {
		s.Fatal("Failed to set Always-on VPN properties: ", err)
	}
	defer func() {
		if err := profile.SetAlwaysOnVPN(cleanupCtx, shillconst.AlwaysOnVPNModeOff, nil); err != nil {
			s.Fatal("Failed to reset Always-on VPN properties: ", err)
		}
	}()

	// Check if always on VPN is set in correct mode.
	props, err := profile.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to get props: ", err)
	}
	if curMode, err := props.GetString(shillconst.ProfilePropertyAlwaysOnVPNMode); err != nil {
		s.Fatal("Failed to get Always-on VPN mode: ", err)
	} else if curMode != vpnMode {
		s.Fatalf("Current Always-on VPN mode is %v, want: %v", curMode, vpnMode)
	}

	// Check if VPN can be automatically connected.
	if err := conn.Service().WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for VPN connected automatically: ", err)
	}

	// Stop the VPN server before restarting ui, otherwise cryptohome will enter a
	// strange state on some boards (e.g., brya).
	if err := conn.Server.Exit(ctx); err != nil {
		s.Fatal("Failed to stop VPN server before logout user: ", err)
	}

	// Restart UI to logout.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to restart ui: ", err)
	}

	// Start the VPN server again. Use vpn.Connection without setting up the
	// service to set up the server only. This code can be refactored once
	// b/257379393 is done.
	vpnServer, err := vpn.NewConnectionWithEnvs(ctx, config, testEnv.BaseServer, nil)
	if err != nil {
		s.Fatal("Failed to prepare VPN server after login again: ", err)
	}
	if err := vpnServer.SetUpWithoutService(ctx); err != nil {
		s.Fatal("Failed to setup VPN server after login again: ", err)
	}
	defer func() {
		if err := vpnServer.Cleanup(cleanupCtx); err != nil {
			s.Fatal("Failed to stop VPN server after the test: ", err)
		}
	}()

	// Re-login to Chrome.
	cr, err = chrome.New(
		ctx,
		chrome.CustomLoginTimeout(chrome.ManagedUserLoginTimeout),
		chrome.KeepState(),     // to avoid resetting TPM
		chrome.FakeLogin(cred), // to use the same user as certs are installed for
	)
	if err != nil {
		s.Fatal("Failed to login again: ", err)
	}
	defer func() {
		if err := cr.Close(ctx); err != nil {
			s.Fatal("Failed to close Chrome connection: ", err)
		}
	}()

	// Manager properties will be cleared after re-login, so we need to disable
	// portal detection again.
	if err := m.SetProperty(ctx, shillconst.ProfilePropertyCheckPortalList, "wifi,cellular"); err != nil {
		s.Fatal("Failed to disable portal detection on ethernet: ", err)
	}

	// Search for the VPN service we set up.
	// In this testing condition, we only have one VPN service, so we can directly
	// search for VPN type to find the service.
	serviceProps := map[string]interface{}{
		shillconst.ServicePropertyType: shillconst.TypeVPN,
	}
	vpnService, err := m.FindMatchingService(ctx, serviceProps)
	if err != nil {
		s.Fatal("Failed to find VPN service: ", err)
	}
	defer func() {
		if err := vpnService.Remove(cleanupCtx); err != nil {
			s.Error("Failed to clean up connection: ", err)
		}
	}()

	// Check if VPN can be automatically connected in 20 seconds.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if connected, err := vpnService.IsConnected(ctx); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get connection state"))
		} else if !connected {
			return errors.Wrap(err, "not connected")
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		s.Error("VPN is not connected automatically: ", err)
	}
}
