// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type vpnUITestCase struct {
	vpnType       vpn.Type
	ipsecAuthType vpn.IPsecAuthType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     VPNUI,
		Desc:     "Follows the user flow to create, connect, disconnect, and forget a VPN service via UI",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "vpnEnvWithCertsAndChromeLoggedIn",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "ikev2_cert",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypeCert,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "ikev2_eap",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypeEAP,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "ikev2_psk",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeIKEv2,
				ipsecAuthType: vpn.AuthTypePSK,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "l2tp_ipsec_cert",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeL2TPIPsec,
				ipsecAuthType: vpn.AuthTypeCert,
			},
		}, {
			Name: "l2tp_ipsec_psk",
			Val: vpnUITestCase{
				vpnType:       vpn.TypeL2TPIPsec,
				ipsecAuthType: vpn.AuthTypePSK,
			},
		}, {
			Name: "openvpn",
			Val: vpnUITestCase{
				vpnType: vpn.TypeOpenVPN,
			},
		}, {
			Name: "wireguard",
			Val: vpnUITestCase{
				vpnType: vpn.TypeWireGuard,
			},
			ExtraSoftwareDeps: []string{"wireguard"},
		}},
	})
}

// vpnClientCertNameInUI is the display name of the client cert we should use in
// the test.
const vpnClientCertNameInUI = "chromelab-wifi-testbed-root.mtv.google.com [chromelab-wifi-testbed-client.mtv.google.com]"

func VPNUI(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(vpn.FixtureEnv).Cr
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	conn, err := apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/internet")
	if err != nil {
		s.Fatal("Failed to open the OS settings page: ", err)
	}
	defer conn.Close()

	testing.ContextLog(ctx, "Setting keyboard layout to English (US)")
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
	defer ew.Close(ctx)

	// Prepares virtualnet environment for the VPN server.
	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology for VPN tests: ", err)
	}
	defer func() {
		if err := networkEnv.TearDown(cleanupCtx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
	}()

	// Prepares VPN server.
	tc := s.Param().(vpnUITestCase)
	config := vpn.NewConfig(
		tc.vpnType,
		vpn.WithIPsecAuthType(tc.ipsecAuthType),
		vpn.WithOpenVPNUseUserPassword(),
		vpn.WithWGUsePSK(true),
		// Enable dual-stack VPN so that 1) we can verify Chrome does not crash with
		// a dual-stack VPN connection; 2) for WireGuard, both IPv4 and IPv6 config
		// can be input properly. Note that not all VPN supports IPv6, IPv4-only VPN
		// will be set up when IPv6 is not supported.
		vpn.WithIPType(vpn.IPTypeIPv4AndIPv6),
	)
	vpnServer, err := vpn.StartServerWithConfig(ctx, networkEnv.Server1, config)
	if err != nil {
		s.Fatal("Failed to create VPN connection: ", err)
	}
	defer vpnServer.Exit(cleanupCtx)

	// Get property values for this VPN connection so that we can fill them in UI.
	vpnProps, err := vpn.CreateProperties(vpnServer, nil /*secondServer*/, config)
	if err != nil {
		s.Fatal("Failed to generate D-Bus properties: ", err)
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Open VPN dialog",
		ui.LeftClick(nodewith.Name("Add network connection").Role(role.Button)),
		ui.LeftClick(nodewith.NameContaining("Add built-in VPN").Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to open VPN dialog: ", err)
	}

	// Inputs VPN properties via UI.
	svcName := "vpn-test-" + tc.vpnType.String()

	// Configures service on the VPN dialog page.
	v := vpnDialogConfigger{ui, ew, tc, vpnProps, svcName}
	if err := v.config(ctx); err != nil {
		s.Fatal("Failed to configure on VPN dialog: ", err)
	}

	// Clicks Connect and checks the "Connected" text on the VPN detail page.
	if err := uiauto.Combine("Connect VPN",
		ui.LeftClick(nodewith.Name("Connect").Role(role.Button)),
		ui.LeftClick(nodewith.Name("VPN").Role(role.Button)),
		ui.LeftClick(nodewith.NameContaining(svcName+", Details")),
		ui.WithTimeout(time.Second*5).WaitUntilExists(nodewith.Name("Connected").Role(role.StaticText)),
	)(ctx); err != nil {
		s.Fatal("Failed to connect VPN: ", err)
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

	// Clicks Disconnect and checks the "Not Connected" text on the page.
	if err := uiauto.Combine("Disconnect VPN",
		ui.LeftClick(nodewith.Name("Disconnect").Role(role.Button)),
		ui.LeftClick(nodewith.Name("Not Connected").Role(role.StaticText)),
	)(ctx); err != nil {
		s.Fatal("Failed to disconnect VPN: ", err)
	}

	// Clicks Forget, it should be navigated to the VPN list page and no service
	// should be shown.
	if err := uiauto.Combine("Forget VPN",
		ui.LeftClick(nodewith.Name("Forget").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name("VPN").Role(role.Heading)),
		ui.Gone(nodewith.NameContaining(svcName)),
	)(ctx); err != nil {
		s.Fatal("Failed to forget VPN: ", err)
	}
}

type vpnDialogConfigger struct {
	ui      *uiauto.Context
	ew      *input.KeyboardEventWriter
	tc      vpnUITestCase
	props   map[string]interface{}
	svcName string
}

func (v *vpnDialogConfigger) inputTextField(ctx context.Context, name, value string) error {
	if err := v.ui.FocusAndWait(nodewith.Name(name).Role(role.TextField))(ctx); err != nil {
		return errors.Wrapf(err, "failed to focus %s", name)
	}
	if err := v.ew.Type(ctx, value); err != nil {
		return errors.Wrapf(err, "failed to input %s", name)
	}
	return nil
}

func (v *vpnDialogConfigger) selectListOption(ctx context.Context, name, value string) error {
	btn := nodewith.Name(name).Role(role.ComboBoxSelect)
	return uiauto.Combine("Select "+name,
		v.ui.WaitUntilExists(btn),
		v.ui.MakeVisible(btn),
		v.ui.LeftClick(btn),
		v.ui.LeftClick(nodewith.Name(value).Role(role.ListBoxOption)),
	)(ctx)
}

func (v *vpnDialogConfigger) config(ctx context.Context) error {
	if err := v.inputTextField(ctx, "Service name", v.svcName); err != nil {
		return err
	}
	switch v.tc.vpnType {
	case vpn.TypeIKEv2:
		return v.configIKEv2(ctx)
	case vpn.TypeL2TPIPsec:
		return v.configL2TPIPsec(ctx)
	case vpn.TypeOpenVPN:
		return v.configOpenVPN(ctx)
	case vpn.TypeWireGuard:
		return v.configWireGuard(ctx)
	default:
		return errors.Errorf("invalid VPN type %s", v.tc.vpnType)
	}
}

func (v *vpnDialogConfigger) configIKEv2(ctx context.Context) error {
	if err := v.selectListOption(ctx, "Provider type", "IPsec (IKEv2)"); err != nil {
		return errors.Wrap(err, "failed to select VPN type")
	}
	if err := v.inputTextField(ctx, "Server hostname", v.props["Provider.Host"].(string)); err != nil {
		return err
	}
	switch v.tc.ipsecAuthType {
	case vpn.AuthTypeCert:
		// Server CA is selected by default.
		if err := v.selectListOption(ctx, "Authentication type", "User certificate"); err != nil {
			return errors.Wrap(err, "failed to select authentication type")
		}
		if err := v.selectListOption(ctx, "User certificate", vpnClientCertNameInUI); err != nil {
			return errors.Wrap(err, "failed to select user certificate")
		}
		if err := v.inputTextField(ctx, "Remote identity (optional)", v.props["IKEv2.RemoteIdentity"].(string)); err != nil {
			return err
		}
	case vpn.AuthTypeEAP:
		// Server CA is selected by default.
		if err := v.selectListOption(ctx, "Authentication type", "Username and password"); err != nil {
			return errors.Wrap(err, "failed to select authentication type")
		}
		if err := v.inputTextField(ctx, "Username", v.props["EAP.Identity"].(string)); err != nil {
			return err
		}
		if err := v.inputTextField(ctx, "Password", v.props["EAP.Password"].(string)); err != nil {
			return err
		}
	case vpn.AuthTypePSK:
		if err := v.selectListOption(ctx, "Authentication type", "Pre-shared key"); err != nil {
			return errors.Wrap(err, "failed to select authentication type")
		}
		if err := v.inputTextField(ctx, "Pre-shared key", v.props["IKEv2.PSK"].(string)); err != nil {
			return err
		}
		if err := v.inputTextField(ctx, "Local identity (optional)", v.props["IKEv2.LocalIdentity"].(string)); err != nil {
			return err
		}
		if err := v.inputTextField(ctx, "Remote identity (optional)", v.props["IKEv2.RemoteIdentity"].(string)); err != nil {
			return err
		}
	default:
		return errors.Errorf("unknown auth type %s", v.tc.ipsecAuthType)
	}
	return nil
}

func (v *vpnDialogConfigger) configL2TPIPsec(ctx context.Context) error {
	if err := v.selectListOption(ctx, "Provider type", "L2TP/IPsec"); err != nil {
		return errors.Wrap(err, "failed to select VPN type")
	}
	if err := v.inputTextField(ctx, "Server hostname", v.props["Provider.Host"].(string)); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Username", v.props["L2TPIPsec.User"].(string)); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Password", v.props["L2TPIPsec.Password"].(string)); err != nil {
		return err
	}

	switch v.tc.ipsecAuthType {
	case vpn.AuthTypeCert:
		// Server CA is selected by default.
		if err := v.selectListOption(ctx, "Authentication type", "User certificate"); err != nil {
			return errors.Wrap(err, "failed to select authentication type")
		}
		if err := v.selectListOption(ctx, "User certificate", vpnClientCertNameInUI); err != nil {
			return errors.Wrap(err, "failed to select user certificate")
		}
	case vpn.AuthTypePSK:
		// Authentication type is default to "Pre-shared key".
		if err := v.inputTextField(ctx, "Pre-shared key", v.props["L2TPIPsec.PSK"].(string)); err != nil {
			return err
		}
	default:
		return errors.Errorf("unknown auth type %s", v.tc.ipsecAuthType)
	}
	return nil
}

func (v *vpnDialogConfigger) configOpenVPN(ctx context.Context) error {
	if err := v.selectListOption(ctx, "Provider type", "OpenVPN"); err != nil {
		return errors.Wrap(err, "failed to select VPN type")
	}
	if err := v.inputTextField(ctx, "Server hostname", v.props["Provider.Host"].(string)); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Username", v.props["OpenVPN.User"].(string)); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Password", v.props["OpenVPN.Password"].(string)); err != nil {
		return err
	}

	// Server CA is selected by default. Only need to select user cert.
	if err := v.selectListOption(ctx, "User certificate", vpnClientCertNameInUI); err != nil {
		return errors.Wrap(err, "failed to select user certificate")
	}
	return nil
}

func (v *vpnDialogConfigger) configWireGuard(ctx context.Context) error {
	if err := v.selectListOption(ctx, "Provider type", "WireGuard"); err != nil {
		return errors.Wrap(err, "failed to select VPN type")
	}

	addrs := v.props["WireGuard.IPAddress"].([]string)
	peer := v.props["WireGuard.Peers"].([]map[string]string)[0]
	if err := v.inputTextField(ctx, "Client IP address", strings.Join(addrs, ",")); err != nil {
		return err
	}
	if err := v.selectListOption(ctx, "Key", "I have a keypair"); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Private key", v.props["WireGuard.PrivateKey"].(string)); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Public key", peer["PublicKey"]); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Preshared key", peer["PresharedKey"]); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Endpoint", peer["Endpoint"]); err != nil {
		return err
	}
	if err := v.inputTextField(ctx, "Allowed IPs", peer["AllowedIPs"]); err != nil {
		return err
	}
	return nil
}
