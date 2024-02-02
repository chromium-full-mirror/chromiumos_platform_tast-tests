// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VPNPolicy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that VPN can correctly be configured from device and user policy",
		Contacts:     []string{"cros-networking@google.com", "taoyl@google.com"},
		BugComponent: "b:1493959",
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Fixture: "chromeEnrolledLoggedIn",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.OpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func VPNPolicy(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	deviceProfileServiceGUID := "Device Policy L2TPIPSec-VPN"
	userProfileServiceGUID := "User Policy L2TPIPSec-VPN"

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology for VPN tests: ", err)
	}
	defer func() {
		if err := networkEnv.TearDown(cleanupCtx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
		// TODO(b/286348339): use a smaller timeout value after b/286348339 is resolved.
		if err := ping.VerifyInternetConnectivity(cleanupCtx, 20*time.Second); err != nil {
			// Only printing log instead of report error here, to avoid lab network issue causing test flakiness.
			testing.ContextLog(cleanupCtx, "Failed to restore Internet connectivity after test: ", err)
		}
	}()

	server, err := vpn.StartServer(ctx, networkEnv.Server1, vpn.TypeL2TPIPsec)
	if err != nil {
		s.Fatal("Failed to start VPN server: ", err)
	}
	defer server.Exit(cleanupCtx)

	testing.ContextLog(ctx, "VPN server started as ", server.UnderlayIP)

	vpnONC := &policy.ONCVPN{
		AutoConnect: false,
		Host:        server.UnderlayIP,
		Type:        "L2TP-IPsec",
		L2TP: &policy.ONCL2TP{
			Username: "chapuser",
			Password: "chapsecret",
		},
		IPsec: &policy.ONCIPsec{
			AuthenticationType: "PSK",
			IKEVersion:         1,
			PSK:                "preshared-key",
		},
	}

	userNetPolicy := &policy.OpenNetworkConfiguration{
		Val: &policy.ONC{
			NetworkConfigurations: []*policy.ONCNetworkConfiguration{
				{
					GUID: userProfileServiceGUID,
					Name: "User Policy L2TPIPSec",
					Type: "VPN",
					VPN:  vpnONC,
				},
			},
		},
	}

	deviceNetPolicy := &policy.DeviceOpenNetworkConfiguration{
		Val: &policy.ONC{
			NetworkConfigurations: []*policy.ONCNetworkConfiguration{
				{
					GUID: deviceProfileServiceGUID,
					Name: "Device Policy L2TPIPSec",
					Type: "VPN",
					VPN:  vpnONC,
				},
			},
		},
	}

	for _, tc := range []struct {
		subtest string
		policy  []policy.Policy
		guid    string
	}{
		{
			subtest: "device",
			policy:  []policy.Policy{deviceNetPolicy},
			guid:    deviceProfileServiceGUID,
		},
		{
			subtest: "user",
			policy:  []policy.Policy{userNetPolicy},
			guid:    userProfileServiceGUID,
		},
	} {
		s.Run(ctx, tc.subtest, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, tc.policy); err != nil {
				s.Fatalf("Failed to update %s policy: %v", tc.subtest, err)
			}

			service, err := vpn.FindVPNService(ctx, m, tc.guid)
			if err != nil {
				s.Fatalf("Failed to find %s service: %v", tc.subtest, err)
			}

			if err := vpn.VerifyVPNServiceConnect(ctx, m, service); err != nil {
				s.Errorf("Failed to verify %s service connectable: %v", tc.subtest, err)
			}
		})
	}
}
