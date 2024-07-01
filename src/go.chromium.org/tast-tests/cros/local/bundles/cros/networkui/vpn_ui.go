// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/networkui/proxysettings"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	vpnManager "go.chromium.org/tast-tests/cros/local/networkui/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type vpnUITestCase struct {
	vpnType       vpn.Type
	ipsecAuthType vpn.IPsecAuthType
	browserType   browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func: VPNUI,
		// This test launches a web page so there should be a lacros variant.
		// Lacros test will be added once the issue (crbug/1366609) is fixed.
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Follows the user flow to create, connect, disconnect, and forget a VPN service via UI, verify the availability of proxy settings for a connected VPN",
		Contacts: []string{
			// These contacts are commented out so that they can be added back in once the tests are stabilized.
			// These are commented out instead of removed outright to avoid losing any contacts in the process.
			// "cros-networking@google.com",
			// "jiejiang@google.com",
			// "cros-connectivity@google.com",
			// "chromeos-connectivity-engprod@google.com",
			// "shijinabraham@google.com",
			// "chadduffin@chromium.org",
			"ryan.liu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Attr:           []string{"group:mainline", "informational"},
		SoftwareDeps:   []string{"chrome"},
		Params: []testing.Param{{
			Name:    "ikev2_cert",
			Fixture: "vpnEnvWithCertsAndChromeLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypeCert,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name:    "ikev2_eap",
			Fixture: "vpnEnvWithCertsAndChromeLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypeEAP,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name:    "ikev2_psk",
			Fixture: "vpnEnvWithCertsAndChromeLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypePSK,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name:    "l2tp_ipsec_cert",
			Fixture: "vpnEnvWithCertsAndChromeLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeL2TPIPsec,
				ipsecAuthType: vpn.AuthTypeCert,
			},
		}, {
			Name:    "l2tp_ipsec_psk",
			Fixture: "vpnEnvWithCertsAndChromeLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeL2TPIPsec,
				ipsecAuthType: vpn.AuthTypePSK,
			},
		}, {
			Name:    "openvpn",
			Fixture: "vpnEnvWithCertsAndChromeLoggedIn",
			Val: vpnUITestCase{
				vpnType: vpn.TypeOpenVPN,
			},
		}, {
			Name:    "wireguard",
			Fixture: "vpnEnvWithCertsAndChromeLoggedIn",
			Val: vpnUITestCase{
				vpnType: vpn.TypeWireGuard,
			},
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name:    "ikev2_cert_non_device_owner",
			Fixture: "vpnEnvWithCertsAndNonDeviceOwnerLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypeCert,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name:    "ikev2_eap_non_device_owner",
			Fixture: "vpnEnvWithCertsAndNonDeviceOwnerLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypeEAP,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name:    "ikev2_eap_guest",
			Fixture: "vpnEnvWithCertsAndGuestLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypeEAP,
				browserType:   browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name:    "ikev2_psk_non_device_owner",
			Fixture: "vpnEnvWithCertsAndNonDeviceOwnerLoggedIn",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypePSK,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}},
	})
}

func VPNUI(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	s.Log("Setting keyboard layout to English (US)")
	imePrefix, err := ime.Prefix(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the ime prefix: ", err)
	}
	if err := ime.AddAndSetInputMethod(ctx, tconn, imePrefix+ime.EnglishUS.ID); err != nil {
		s.Fatal("Failed to set keyboard to en-US: ", err)
	}

	ew, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer ew.Close(cleanupCtx)

	certVals := s.FixtValue().(vpn.FixtureEnv).CertVals
	if certVals == nil {
		s.Fatal("Failed to obtain an valid certificate values")
	}

	// Prepares VPN server.
	tc := s.Param().(vpnUITestCase)
	config := vpn.NewConfig(
		tc.vpnType,
		vpn.WithIPsecAuthType(tc.ipsecAuthType),
		vpn.WithOpenVPNUseUserPassword(),
		vpn.WithWGUsePSK(true),
		vpn.WithCertVals(certVals),
		// Enable dual-stack VPN so that 1) we can verify Chrome does not crash with
		// a dual-stack VPN connection; 2) for WireGuard, both IPv4 and IPv6 config
		// can be input properly. Note that not all VPN supports IPv6, IPv4-only VPN
		// will be set up when IPv6 is not supported.
		vpn.WithIPType(vpn.IPTypeIPv4AndIPv6),
	)

	svcName := "vpn-test-" + tc.vpnType.String()
	vpnServer, vpnHelper, cleanup, err := vpnManager.NewDialogHelperWithVPNServer(ctx, config, svcName)
	if err != nil {
		s.Fatal("Failed to start VPN server: ", err)
	}
	defer cleanup(cleanupCtx)

	settings, err := ossettings.OpenJoinVPNDialog(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open VPN dialog: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "vpn_settings_ui_dump")

	// Configures service on the VPN dialog page.
	if err := vpnHelper.FillInConfigurations(ctx, cr, tconn, ew); err != nil {
		s.Fatal("Failed to configure on VPN dialog: ", err)
	}

	// Clicks Connect and checks the "Connected" text on the VPN detail page.
	if err := vpnHelper.ConnectAndWait(ctx, tconn); err != nil {
		s.Fatal("Failed to connect to VPN: ", err)
	}

	// Pings server gateway to make sure VPN is connected. This is required since
	// some VPN services (e.g., WireGuard) will show connected even if we have a
	// wrong configuration.
	reachableIPs := []string{vpnServer.OverlayIPv4}
	if len(vpnServer.OverlayIPv6) > 0 {
		reachableIPs = append(reachableIPs, vpnServer.OverlayIPv6)
	}
	for _, ip := range reachableIPs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, ip, "chronos", 10*time.Second); err != nil {
			s.Errorf("Failed to ping %s: %v", ip, err)
		}
	}

	if err := verifyProxySettingsAvailability(ctx, cr, tconn, ew, s.OutDir(), svcName); err != nil {
		s.Fatal("Failed to verify proxy settings is available: ", err)
	}

	// Ensuring settings apps is restored back to after verifying proxy settings since
	// the `proxysettings.Manager` is used to verify proxy settings and the settings app
	// will be closed with it.
	if _, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, svcName, netconfigtypes.VPN); err != nil {
		s.Fatalf("Failed to open the %v vpn network detail page: %v", svcName, err)
	}

	// Clicks Disconnect and checks the "Not Connected" text on the page.
	if err := ossettings.DisconnectVPN(ctx, tconn); err != nil {
		s.Fatal("Failed to disconnect VPN: ", err)
	}

	// Clicks Forget, it should be navigated to the VPN list page and no service
	// should be shown.
	if err := ossettings.ForgetVPN(ctx, tconn, svcName); err != nil {
		s.Fatal("Failed to forget VPN: ", err)
	}
}

// verifyProxySettingsAvailability verifies that the proxy settings of a VPN
// network are available and can be populated. The proxy should be able to set
// and save, they should correctly display and can be editable.
func verifyProxySettingsAvailability(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, outDir, vpnName string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	manager := proxysettings.NewProxySettingsManager(proxysettings.LoggedIn)
	proxyValues := []*proxysettings.Config{
		{
			Protocol: proxysettings.HTTP,
			Host:     "localhost",
			Port:     "123",
		}, {
			Protocol: proxysettings.HTTPS,
			Host:     "localhost",
			Port:     "456",
		}, {
			Protocol: proxysettings.Socks,
			Host:     "socks5://localhost",
			Port:     "8080",
		},
	}

	// Set and save the proxy info.
	// Isolate the step to leverage `defer` pattern.
	if err := func(ctx context.Context) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
		defer cancel()

		if err := manager.Launch(ctx, cr, tconn, proxysettings.VPN(vpnName)); err != nil {
			return errors.Wrap(err, "failed to launch proxy settings instance")
		}
		defer manager.Close(cleanupCtx, cr, tconn)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, outDir, func() bool { return retErr != nil }, cr, "proxy_settings_for_vpn")

		return manager.SetManualConfig(ctx, tconn, kb, proxyValues)
	}(ctx); err != nil {
		return err
	}

	// Reopen the proxy settings to check whether the proxy info is saved correctly.
	if err := manager.Launch(ctx, cr, tconn, proxysettings.VPN(vpnName)); err != nil {
		return errors.Wrap(err, "failed to launch proxy settings instance")
	}
	defer manager.Close(cleanupCtx, cr, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, outDir, func() bool { return retErr != nil }, cr, "proxy_settings_for_vpn_after_reopen")

	for _, pv := range proxyValues {
		if resultPv, err := manager.ManualConfigContent(ctx, tconn, pv.Protocol); err != nil {
			return errors.Wrapf(err, "failed to get proxy value for %q", pv.Protocol.Name())
		} else if !reflect.DeepEqual(resultPv, pv) {
			return errors.Errorf("failed to verify proxy value for %q: got %q, want %q", pv.Protocol.Name(), resultPv, pv)
		}
	}

	return nil
}
