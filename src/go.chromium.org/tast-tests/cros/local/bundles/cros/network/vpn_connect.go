// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"strings"
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

	// Xauth is required in an L2TP/IPsec connection, but user is not configured.
	ipsecXauthMissingUser bool
	// Xauth is required in an L2TP/IPsec connection, but password is wrong.
	ipsecXauthWrongUser bool
	// Cert verify is required in an OpenVPN connection, but the configured hash
	// is wrong.
	openVPNCertVerifyWrongHash bool
	// Cert verify is required in an OpenVPN connection, but the configured
	// subject is wrong.
	openVPNCertVerifyWrongSubject bool
	// Cert verify is required in an OpenVPN connection, but the configured CN is
	// wrong.
	openVPNCertVerifyWrongCN bool
	// Let shill generate the key pair in an WireGuard VPN.
	wgGenKey bool
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
					vpn.WithL2TPIPsecXAuth(),
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
					vpn.WithL2TPIPsecXAuth(),
				},
				ipsecXauthMissingUser: true,
				shouldFail:            true,
			},
			Fixture:   "vpnEnv",
			ExtraAttr: []string{"group:network", "network_platform"},
		}, {
			Name: "l2tp_ipsec_psk_xauth_wrong_user",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeL2TPIPsec,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypePSK),
					vpn.WithL2TPIPsecXAuth(),
				},
				ipsecXauthWrongUser: true,
				shouldFail:          true,
			},
			Fixture:   "vpnEnv",
			ExtraAttr: []string{"group:network", "network_platform"},
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
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifySubject),
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
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifySubject),
				},
				openVPNCertVerifyWrongHash: true,
				shouldFail:                 true,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:network", "network_platform"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_cert_verify_wrong_subject",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifySubject),
				},
				openVPNCertVerifyWrongSubject: true,
				shouldFail:                    true,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:network", "network_platform"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_cert_verify_wrong_cn",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeOpenVPN,
				opts: []vpn.Option{
					vpn.WithIPsecAuthType(vpn.AuthTypeCert),
					vpn.WithOpenVPNCertVerify(vpn.OpenVPNCertVerifyCNOnly),
				},
				openVPNCertVerifyWrongCN: true,
				shouldFail:               true,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:network", "network_platform"},
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
			ExtraAttr:         []string{"group:network", "network_platform"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "wireguard_psk",
			Val: vpnConnectTestParams{
				vpnType: vpn.TypeWireGuard,
				opts: []vpn.Option{
					vpn.WithWGUsePSK(true),
				},
			},
			Fixture:           "vpnEnv",
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name: "wireguard_generate_key",
			Val: vpnConnectTestParams{
				vpnType:  vpn.TypeWireGuard,
				wgGenKey: true,
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

	// Start VPN server.
	opts := append([]vpn.Option{
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
		vpn.WithoutAutoConnect(),
	}, tc.opts...)

	// WG server will need the public key of the client, so generate the key pair
	// before that.
	if tc.wgGenKey {
		pubKey, err := vpn.GenerateWireGuardKey(ctx)
		if err != nil {
			s.Fatal("Failed to generate wireguard key: ", err)
		}
		opts = append(opts, vpn.WithWGClientPublicKey(pubKey))
	}

	server, err := vpn.StartServer(ctx, networkEnv.Server1, tc.vpnType, opts...)
	if err != nil {
		s.Fatal("Failed to start VPN server: ", err)
	}
	defer func() {
		if err := server.Exit(cleanupCtx); err != nil {
			s.Error("Failed to exit VPN server: ", err)
		}
	}()

	// Create shill property dict to connect to server.
	props, err := vpn.CreateProperties(server, nil /*secondServer*/)
	if err != nil {
		s.Fatal("Failed to create VPN properties: ", err)
	}

	// Modify the dict according to the test case.
	if tc.ipsecXauthMissingUser {
		delete(props, "L2TPIPsec.XauthUser")
		delete(props, "L2TPIPsec.XauthPassword")
	}
	if tc.ipsecXauthWrongUser {
		props["L2TPIPsec.XauthUser"] = "wrong-user"
		props["L2TPIPsec.XauthPassword"] = "wrong-password"
	}
	if tc.openVPNCertVerifyWrongHash {
		props["OpenVPN.VerifyHash"] = "00" + strings.Repeat(":00", 19)
	}
	if tc.openVPNCertVerifyWrongSubject {
		props["OpenVPN.VerifyX509Name"] = "bogus subject name"
	}
	if tc.openVPNCertVerifyWrongCN {
		props["OpenVPN.VerifyX509Name"] = "bogus cn"
		props["OpenVPN.VerifyX509Type"] = "name"
	}

	// Configure the service.
	service, err := vpn.ConfigureServiceWithProps(ctx, props)
	if err != nil {
		s.Fatal("Failed to configure VPN service: ", err)
	}
	defer func() {
		if err := service.Remove(cleanupCtx); err != nil {
			s.Error("Failed to remove VPN service: ", err)
		}
	}()

	testing.ContextLog(ctx, "Connecting to the service: ", service)
	if err := service.Connect(ctx); err != nil {
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
	if err := service.WaitForPropertyInSetWithOptions(ctx,
		shillconst.ServicePropertyState,
		append(shillconst.ServiceConnectedStates, shillconst.ServiceStateFailure),
		&testing.PollOptions{Timeout: connectTimeout},
	); err != nil {
		s.Fatal("Failed to wait for VPN service state changed: ", err)
	}

	if err := dumputil.DumpNetworkInfo(ctx, "network_dump_after_vpn_connect.txt"); err != nil {
		testing.ContextLog(ctx, "Failed to dump network info after VPN connect")
	}

	connected, err := service.IsConnected(ctx)
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
	if err := ping.ExpectPingSuccessWithTimeout(ctx, server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s: %v", server.OverlayIPv4, err)
	}
}
