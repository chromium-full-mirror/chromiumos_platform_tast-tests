// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	certManager "go.chromium.org/tast-tests/cros/local/networkui/certificate"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type vpnPolicyTestCase struct {
	isDevicePolicy bool
	vpnType        vpn.Type
	serverOptions  []vpn.Option
}

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
		Params: []testing.Param{
			{
				Name: "l2tp_ipsec",
				Val: vpnPolicyTestCase{
					isDevicePolicy: false,
					vpnType:        vpn.TypeL2TPIPsec,
				},
			},
			{
				Name: "l2tp_ipsec_device_policy",
				Val: vpnPolicyTestCase{
					isDevicePolicy: true,
					vpnType:        vpn.TypeL2TPIPsec,
				},
			},
			{
				Name: "openvpn",
				Val: vpnPolicyTestCase{
					isDevicePolicy: false,
					vpnType:        vpn.TypeOpenVPN,
					serverOptions: []vpn.Option{
						vpn.WithOpenVPNUseUserPassword(),
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
			},
		},
	})
}

func VPNPolicy(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

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

	tc := s.Param().(vpnPolicyTestCase)

	serverOpts := tc.serverOptions
	server, err := vpn.StartServer(ctx, networkEnv.Server1, tc.vpnType, serverOpts...)
	if err != nil {
		s.Fatal("Failed to start VPN server: ", err)
	}
	defer server.Exit(cleanupCtx)

	testing.ContextLog(ctx, "VPN server started as ", server.UnderlayIP)

	// A random-generated GUID. Need to be consistent in VPN ONC and Cert ONC.
	const serverCACertGUID = "{b3aae353-cfa9-4093-9aff-9f8ee2bf8c29}"
	vpnONC := &policy.ONCVPN{}
	switch tc.vpnType {
	case vpn.TypeL2TPIPsec:
		vpnONC = &policy.ONCVPN{
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
	case vpn.TypeOpenVPN:
		vpnONC = &policy.ONCVPN{
			AutoConnect: false,
			Host:        server.UnderlayIP,
			Type:        "OpenVPN",
			OpenVPN: &policy.ONCOpenVPN{
				ClientCertType: "Pattern",
				ClientCertPattern: &policy.ONCClientCertPattern{
					Issuer: &policy.ONCSubjectPattern{
						CommonName: "chromelab-wifi-testbed-root.mtv.google.com",
					},
				},
				UserAuthenticationType: "Password",
				// `openvpnUsername` and `openvpnPassword` in vpn/server.go.
				Username: "username",
				Password: "password",
				ServerCARefs: []string{
					serverCACertGUID,
				},
			},
		}
	default:
		s.Fatalf("Unsupported VPN type %s", tc.vpnType)
	}

	serviceGUID := s.TestName() + "_guid"
	onc := &policy.ONC{
		Certificates: []*policy.ONCCertificate{
			{
				GUID:      serverCACertGUID,
				TrustBits: []string{"Web"},
				Type:      "Authority",
				X509:      string(certificate.TestCert1().CACred.Cert),
			},
		},
		NetworkConfigurations: []*policy.ONCNetworkConfiguration{
			{
				GUID: serviceGUID,
				Name: "Policy VPN" + s.TestName(),
				Type: "VPN",
				VPN:  vpnONC,
			},
		},
	}

	var netPolicy policy.Policy
	if tc.isDevicePolicy {
		netPolicy = &policy.DeviceOpenNetworkConfiguration{
			Val: onc,
		}
	} else {
		netPolicy = &policy.OpenNetworkConfiguration{
			Val: onc,
		}
	}

	// Insert client certificate (into user slot).
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	certManager.CreateCertAndImport(
		ctx,
		cr,
		tconn,
		browser.TypeAsh,
		certificate.TestCert1(),
		certManager.TypeImportAndBind,
		"", /* password */
		0,  /* trust settings for the CA certificate */
	)
	defer certManager.DeleteCert(
		tconn,
		cr.Browser(),
		certManager.NewCertData(certificate.TestCert1(), certManager.TypeClient),
		certManager.NewCertData(certificate.TestCert1(), certManager.TypeCA),
	)(cleanupCtx)

	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{netPolicy}); err != nil {
		s.Fatal("Failed to update policy: ", err)
	}
	// GoBigSleepLint: Time for Chrome to pick up client certificate based on
	// ClientCertPattern and apply to shill service. There is no signal to poll
	// for the readiness of this operation.
	testing.Sleep(ctx, 5*time.Second)

	service, err := vpn.FindVPNService(ctx, m, serviceGUID)
	if err != nil {
		s.Fatal("Failed to find service: ", err)
	}

	if err := vpn.VerifyVPNServiceConnect(ctx, m, service); err != nil {
		s.Error("Failed to verify service connectable: ", err)
	}

}
