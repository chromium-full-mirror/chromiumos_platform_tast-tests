// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/network/vpn"
	"chromiumos/tast/local/network/dumputil"
	"chromiumos/tast/local/network/routing"
	"chromiumos/tast/testing"
)

type vpnTestParams struct {
	config     vpn.Config
	shouldFail bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     VPNConnect,
		Desc:     "Ensure that we can connect to a VPN under different configurations",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		// We use different fixtures in different tests, based on whether they need
		// certificates or not. Note that for tests of IKEv2 and L2TP/IPsec with
		// certs, although they do not involve Chrome by intention, but Chrome may
		// change the cert properties of them proactively, and thus we need Chrome
		// is logged-in as the same user with our fake TPM. Also see
		// b/192425378#comment5.
		Params: []testing.Param{{
			Name: "ikev2_psk",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:     vpn.TypeIKEv2,
					AuthType: vpn.AuthTypePSK,
				},
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "ikev2_cert",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:     vpn.TypeIKEv2,
					IPType:   vpn.IPTypeIPv4,
					AuthType: vpn.AuthTypeCert,
				},
			},
			Fixture:           "vpnEnvWithCertsAndChromeLoggedIn",
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "ikev2_eap_mschapv2",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:     vpn.TypeIKEv2,
					IPType:   vpn.IPTypeIPv4,
					AuthType: vpn.AuthTypeEAP,
				},
			},
			Fixture:           "vpnEnvWithCertsAndChromeLoggedIn",
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "l2tp_ipsec_psk",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:     vpn.TypeL2TPIPsec,
					AuthType: vpn.AuthTypePSK,
				},
			},
			Fixture: "vpnEnv",
		}, {
			Name: "l2tp_ipsec_psk_xauth",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:          vpn.TypeL2TPIPsec,
					AuthType:      vpn.AuthTypePSK,
					IPsecUseXauth: true,
				},
			},
			Fixture: "vpnEnv",
		}, {
			Name: "l2tp_ipsec_psk_xauth_missing_user",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:                  vpn.TypeL2TPIPsec,
					AuthType:              vpn.AuthTypePSK,
					IPsecUseXauth:         true,
					IPsecXauthMissingUser: true,
				},
				shouldFail: true,
			},
			Fixture: "vpnEnv",
		}, {
			Name: "l2tp_ipsec_psk_xauth_wrong_user",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:                vpn.TypeL2TPIPsec,
					AuthType:            vpn.AuthTypePSK,
					IPsecUseXauth:       true,
					IPsecXauthWrongUser: true,
				},
				shouldFail: true,
			},
			Fixture: "vpnEnv",
		}, {
			Name: "l2tp_ipsec_cert",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:     vpn.TypeL2TPIPsec,
					AuthType: vpn.AuthTypeCert,
				},
			},
			Fixture: "vpnEnvWithCertsAndChromeLoggedIn",
		}, {
			Name: "openvpn",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:           vpn.TypeOpenVPN,
					AuthType:       vpn.AuthTypeCert,
					OpenVPNTLSAuth: true,
				},
			},
			Fixture: "vpnEnvWithCerts",
		}, {
			Name: "openvpn_user_pass",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:                   vpn.TypeOpenVPN,
					AuthType:               vpn.AuthTypeCert,
					OpenVPNUseUserPassword: true,
				},
			},
			Fixture: "vpnEnvWithCerts",
		}, {
			Name: "openvpn_cert_verify",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:              vpn.TypeOpenVPN,
					AuthType:          vpn.AuthTypeCert,
					OpenVPNCertVerify: true,
				},
			},
			Fixture: "vpnEnvWithCerts",
		}, {
			Name: "openvpn_cert_verify_wrong_hash",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:                       vpn.TypeOpenVPN,
					AuthType:                   vpn.AuthTypeCert,
					OpenVPNCertVerify:          true,
					OpenVPNCertVerifyWrongHash: true,
				},
				shouldFail: true,
			},
			Fixture: "vpnEnvWithCerts",
		}, {
			Name: "openvpn_cert_verify_wrong_subject",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:                          vpn.TypeOpenVPN,
					AuthType:                      vpn.AuthTypeCert,
					OpenVPNCertVerify:             true,
					OpenVPNCertVeirfyWrongSubject: true,
				},
				shouldFail: true,
			},
			Fixture: "vpnEnvWithCerts",
		}, {
			Name: "openvpn_cert_verify_wrong_cn",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:                     vpn.TypeOpenVPN,
					AuthType:                 vpn.AuthTypeCert,
					OpenVPNCertVerify:        true,
					OpenVPNCertVerifyWrongCN: true,
				},
				shouldFail: true,
			},
			Fixture: "vpnEnvWithCerts",
		}, {
			Name: "openvpn_cert_verify_cn_only",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:                    vpn.TypeOpenVPN,
					AuthType:                vpn.AuthTypeCert,
					OpenVPNCertVerify:       true,
					OpenVPNCertVerifyCNOnly: true,
				},
			},
			Fixture: "vpnEnvWithCerts",
		}, {
			Name: "wireguard",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:   vpn.TypeWireGuard,
					IPType: vpn.IPTypeIPv4,
				},
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name: "wireguard_psk",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:     vpn.TypeWireGuard,
					IPType:   vpn.IPTypeIPv4,
					AuthType: vpn.AuthTypePSK,
				},
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name: "wireguard_generate_key",
			Val: vpnTestParams{
				config: vpn.Config{
					Type:         vpn.TypeWireGuard,
					IPType:       vpn.IPTypeIPv4,
					WGAutoGenKey: true,
				},
			},
			Fixture:           "vpnEnv",
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
	routingEnv := routing.NewTestEnvWithoutResetProfile()
	if err := routingEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to setup routing env: ", err)
	}
	defer func() {
		if err := routingEnv.TearDown(cleanupCtx); err != nil {
			testing.ContextLog(ctx, "Failed to tear down routing env: ", err)
		}
	}()

	config := s.Param().(vpnTestParams).config
	config.CertVals = s.FixtValue().(vpn.FixtureEnv).CertVals
	conn, err := vpn.NewConnectionWithEnvs(ctx, config, routingEnv.BaseServer, routingEnv.BaseRouter)
	if err != nil {
		s.Fatal("Failed to create connection object: ", err)
	}

	defer func() {
		if err := conn.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up connection: ", err)
		}
	}()

	if err := conn.SetUp(ctx); err != nil {
		s.Fatal("Failed to setup VPN server: ", err)
	}
	connected, err := conn.Connect(ctx)
	shouldFail := s.Param().(vpnTestParams).shouldFail
	if err := dumputil.DumpNetworkInfo(ctx, "network_dump_after_vpn_connect.txt"); err != nil {
		testing.ContextLog(ctx, "Failed to dump network info after VPN connect")
	}
	if err != nil {
		s.Fatal("Failed to connect to VPN server: ", err)
	} else if !connected && !shouldFail {
		s.Fatal("Failed to connect to VPN server: the service state changed to failure")
	} else if connected && shouldFail {
		s.Fatal("Connect to VPN server should fail")
	} else if !connected && shouldFail {
		return
	}

	// Do a simple ping check to make sure we are really connected.
	if err := routing.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s: %v", conn.Server.OverlayIPv4, err)
	}
}
