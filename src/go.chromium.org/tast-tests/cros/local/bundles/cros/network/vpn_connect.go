// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type vpnConnectTestParams struct {
	vpnType    vpn.Type
	opts       []vpn.Option
	shouldFail bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     VPNConnect,
		Desc:     "Ensure that we can connect to a VPN under different configurations",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		LacrosStatus: testing.LacrosVariantUnneeded,
		// We use different fixtures in different tests, based on whether they need
		// certificates or not. The following configurations are covered by
		// VPNRouting tests so we don't need them here.
		// - IKEv2 with PSK;
		// - L2TP/IPsec with PSK;
		// - wireguard without PSK.
		Params: []testing.Param{{
			Name: "ikev2_cert",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeIKEv2,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
				},
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:mainline"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "ikev2_eap_mschapv2",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeIKEv2,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeEAP),
				},
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:mainline"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "l2tp_ipsec_psk_xauth",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeL2TPIPsec,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypePSK),
					vpn.WithL2TPIPsecXAuth(vpn.L2TPIPsecXauthCorrect),
				},
			},
			Fixture:   "vpnEnv",
			ExtraAttr: []string{"group:mainline"},
		}, {
			Name: "l2tp_ipsec_psk_xauth_missing_user",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeL2TPIPsec,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypePSK),
					vpn.WithL2TPIPsecXAuth(vpn.L2TPIPsecXauthMissingUser),
				},
				shouldFail: true,
			},
			Fixture:   "vpnEnv",
			ExtraAttr: []string{"group:network", "network_platform_unstable"},
		}, {
			Name: "l2tp_ipsec_psk_xauth_wrong_user",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeL2TPIPsec,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypePSK),
					vpn.WithL2TPIPsecXAuth(vpn.L2TPIPsecXauthWrongUser),
				},
				shouldFail: true,
			},
			Fixture:   "vpnEnv",
			ExtraAttr: []string{"group:network", "network_platform_unstable"},
		}, {
			Name: "l2tp_ipsec_cert",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeL2TPIPsec,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
				},
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:mainline"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNTLSAuth(),
				},
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:mainline"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_user_pass",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNUseUserPassword(),
				},
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:mainline"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_cert_verify",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifyCorrect),
				},
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:mainline"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_cert_verify_wrong_hash",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifyWrongHash),
				},
				shouldFail: true,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:network", "network_platform_unstable"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_cert_verify_wrong_subject",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifyWrongSubject),
				},
				shouldFail: true,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:network", "network_platform_unstable"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_cert_verify_wrong_cn",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifyWrongCN),
				},
				shouldFail: true,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:network", "network_platform_unstable"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_cert_verify_cn_only",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifyCNOnly),
				},
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:network", "network_platform_unstable"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "wireguard_psk",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeWireGuard,
				opts: []vpn.Option{
					vpn.WithWGUsePSK(),
				},
			},
			Fixture:           "vpnEnv",
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name: "wireguard_generate_key",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeWireGuard,
				opts: []vpn.Option{
					vpn.WithWGAutoGenKey(),
				},
			},
			Fixture:           "vpnEnv",
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"wireguard"},
		}},
	})
}

func VPNConnect(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Create envs for holding servers.
	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology for VPN tests: ", err)
	}
	defer func() {
		if err := networkEnv.TearDown(cleanupCtx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
	}()

	tc := s.Param().(vpnConnectTestParams)
	opts := append([]vpn.Option{
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
		vpn.WithoutAutoConnect(),
	}, tc.opts...)
	conn, err := vpn.StartConnection(ctx, networkEnv.Server1, tc.vpnType, opts...)
	if err != nil {
		s.Fatal("Failed to create VPN connection: ", err)
	}
	defer func() {
		if err := conn.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up VPN connection: ", err)
		}
	}()

	testing.ContextLog(ctx, "Connecting to the service: ", conn.Service())
	if err := conn.Service().Connect(ctx); err != nil {
		s.Fatal("Failed to call Connect on the service: ", err)
	}

	// Waits until connection established or failed. Some failures can only be
	// detected on a timeout so use a large timeout value for failure cases, e.g.,
	// some failures for IPsec-based VPN are detected based on timeout, which is
	// 30 seconds at maximum for all the current test cases.
	connectTimeout := 10 * time.Second
	if tc.shouldFail {
		connectTimeout = 35 * time.Second
	}
	if err := conn.Service().WaitForPropertyInSetWithOptions(ctx,
		shillconst.ServicePropertyState,
		append(shillconst.ServiceConnectedStates, shillconst.ServiceStateFailure),
		&testing.PollOptions{Timeout: connectTimeout},
	); err != nil {
		s.Fatal("Failed to wait for VPN service state changed: ", err)
	}

	if err := dumputil.DumpNetworkInfo(ctx, "network_dump_after_vpn_connect.txt"); err != nil {
		testing.ContextLog(ctx, "Failed to dump network info after VPN connect")
	}

	connected, err := conn.Service().IsConnected(ctx)
	if err != nil {
		s.Fatal("Failed to get connected state of the service")
	}

	if tc.shouldFail {
		if connected {
			s.Fatal("VPN service is connected unexpectedly")
		}
		return
	}

	if !connected {
		s.Fatal("VPN service state changed to failure")
	}
	// Do a simple ping check to make sure we are really connected.
	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s: %v", conn.Server.OverlayIPv4, err)
	}
}
