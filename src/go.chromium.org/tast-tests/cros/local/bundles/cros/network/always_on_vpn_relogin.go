// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pkcs11/netcertstore"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// alwaysOnVPNReloginTestCase defines mode and config of the VPN
// we want to set up in the test
type alwaysOnVPNReloginTestCase struct {
	mode    string // mode of always-on VPN we want to test.
	vpnType vpn.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AlwaysOnVPNRelogin,
		Desc:         "Host VPN client can be configured as always-on VPN and connected automatically after logout and login",
		Contacts:     []string{"cros-networking@google.com", "chuweih@google.com"},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Fixture:      "vpnEnvWithCerts",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "strict_mode_l2tp_ipsec",
				Val: alwaysOnVPNReloginTestCase{
					mode:    shillconst.AlwaysOnVPNModeStrict,
					vpnType: vpn.TypeL2TPIPsec,
				},
			},
			{
				Name: "best_effort_mode_l2tp_ipsec",
				Val: alwaysOnVPNReloginTestCase{
					mode:    shillconst.AlwaysOnVPNModeBestEffort,
					vpnType: vpn.TypeL2TPIPsec,
				},
			},
			{
				Name: "strict_mode_openvpn",
				Val: alwaysOnVPNReloginTestCase{
					mode:    shillconst.AlwaysOnVPNModeStrict,
					vpnType: vpn.TypeOpenVPN,
				},
			},
			{
				Name: "best_effort_mode_openvpn",
				Val: alwaysOnVPNReloginTestCase{
					mode:    shillconst.AlwaysOnVPNModeBestEffort,
					vpnType: vpn.TypeOpenVPN,
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
	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology for VPN tests: ", err)
	}
	defer func() {
		if err := networkEnv.TearDown(cleanupCtx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
	}()

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

	// Set up VPN server and shill profile based on the config.
	config := vpn.NewConfig(
		s.Param().(alwaysOnVPNReloginTestCase).vpnType,
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
	)

	server, err := vpn.StartServerWithConfig(ctx, networkEnv.Server1, config)
	if err != nil {
		s.Fatal("Failed to start VPN server: ", err)
	}
	defer func() {
		if server == nil {
			return
		}
		if err := server.Exit(cleanupCtx); err != nil {
			s.Log("Failed to clean up VPN server: ", err)
		}
	}()

	service, err := vpn.ConfigureService(ctx, server, nil)
	if err != nil {
		s.Fatal("Failed to create VPN service: ", err)
	}
	defer func() {
		if err := service.Remove(cleanupCtx); err != nil {
			s.Log("Failed to remove VPN service in shill: ", err)
		}
	}()

	// Use set up host VPN as service and change the Always-on VPN mode.
	vpnMode := s.Param().(alwaysOnVPNReloginTestCase).mode
	s.Logf("Setting always-on-vpn mode in shill to %s and waiting for VPN connected", vpnMode)
	profile, err := m.ActiveProfile(ctx)
	if err != nil {
		s.Fatal("Failed to get active profile: ", err)
	}
	if err := profile.SetAlwaysOnVPN(ctx, vpnMode, service); err != nil {
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
	if err := service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for VPN connected automatically: ", err)
	}

	// Stop the VPN server before restarting ui, otherwise cryptohome will enter a
	// strange state on some boards (e.g., brya).
	if err := server.Exit(ctx); err != nil {
		s.Fatal("Failed to stop VPN server before logout user: ", err)
	}
	// Reset server variable here to avoid the defer function above to be call
	// again on the same server object.
	server = nil

	s.Log("VPN has been connected. Re-login Chrome and then check the VPN state again")

	// Restart UI to logout.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to restart ui: ", err)
	}

	// Defer to cleanup is set up above.
	server, err = vpn.StartServerWithConfig(ctx, networkEnv.Server1, config)
	if err != nil {
		s.Fatal("Failed to start VPN server after re-login: ", err)
	}

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
