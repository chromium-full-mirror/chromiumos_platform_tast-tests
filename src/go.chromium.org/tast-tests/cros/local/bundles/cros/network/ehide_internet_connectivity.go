// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type ehideInternetConnectivityTestCase struct {
	vpn        bool // whether setting up a VPN
	indefinite bool // whether having a long timeout at the end or not
}

const ehideInternetConnectivityIndefiniteTimeout = 10 * time.Hour

func init() {
	testing.AddTest(&testing.Test{
		Func:         EhideInternetConnectivity,
		Desc:         "Verify that there is no Ethernet connection when ehide has started",
		Contacts:     []string{"cros-networking@google.com", "jiejiang@google.com"},
		BugComponent: "b:1493959", // ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		Fixture:      "ehide",
		Params: []testing.Param{{
			Val: ehideInternetConnectivityTestCase{
				vpn:        false,
				indefinite: false,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "vpn",
			Val: ehideInternetConnectivityTestCase{
				vpn:        true,
				indefinite: false,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			// This is NOT a test. Just for setting up a virtual network environment
			// with VPN on the DUT.
			Name: "indefinite",
			Val: ehideInternetConnectivityTestCase{
				vpn:        true,
				indefinite: true,
			},
			Timeout: ehideInternetConnectivityIndefiniteTimeout,
		}},
	})
}

func EhideInternetConnectivity(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tc := s.Param().(ehideInternetConnectivityTestCase)

	if !tc.indefinite {
		hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
			testhooks.NewSaveNetLogHook(),
			testhooks.NewTcpdumpHook(),
			testhooks.NewDumpHostOnFailureHook(),
		)
		if err != nil {
			s.Fatal("Failed to run network test hooks: ", err)
		}
		s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
		defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)
	}

	env, err := virtualnet.CreateRouterEnvWithInternet(ctx, subnet.NewPool())
	if err != nil {
		s.Fatal("Failed to create virtualnet env: ", err)
	}
	defer func() {
		if err := env.Cleanup(cleanupCtx); err != nil {
			s.Log("Failed to cleanup virtualnet env: ", err)
		}
	}()

	s.Log("Waiting for shill online")
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}
	expectProps := map[string]interface{}{
		shillconst.ServicePropertyState: shillconst.ServiceStateOnline,
	}
	// In the lab network, there is a chance that the http(s) request for portal
	// detection fails. Since a single trial will take 10 seconds for the
	// timeout, allow retry for multiple times here.
	if _, err := m.WaitForServiceProperties(ctx, expectProps, 40*time.Second); err != nil {
		s.Fatal("Failed to wait for shill online: ", err)
	}

	if tc.vpn {
		s.Log("Setting up VPN connection")
		vpnConn, err := vpn.StartConnection(ctx, env.Env, vpn.TypeIKEv2)
		if err != nil {
			s.Fatal("Failed to set up VPN connection: ", err)
		}
		defer func() {
			if err := vpnConn.Cleanup(cleanupCtx); err != nil {
				s.Log("Failed to clean up VPN connection: ", err)
			}
		}()
	}

	if err := ping.VerifyInternetConnectivity(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to verify Internet connectivity: ", err)
	}

	if tc.indefinite {
		s.Logf("Environment setup done. Sleeping for %v before the cleanup", ehideInternetConnectivityIndefiniteTimeout)
		s.Log("Hit Ctrl+C and then reboot to quit this environment at any time")
		// GoBigSleepLint: This is not a test. Several daemons are forked from the
		// tast process so we have to keep the test running to make sure they are
		// not killed.
		testing.Sleep(ctx, ehideInternetConnectivityIndefiniteTimeout)
	}
}
