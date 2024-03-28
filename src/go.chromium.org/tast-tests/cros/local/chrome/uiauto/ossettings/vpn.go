// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/dropdown"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DisconnectVPN disconnects from a VPN network and ensure it's disconnected.
// This utility can only work when the detail page of the desired VPN network is opened.
func DisconnectVPN(ctx context.Context, tconn *chrome.TestConn) error {
	settings := New(tconn)
	return uiauto.Combine("disconnect VPN",
		settings.LeftClick(nodewith.Name("Disconnect").Role(role.Button)),
		settings.WaitUntilExists(nodewith.Name("Not Connected").Role(role.StaticText)),
	)(ctx)
}

// ForgetVPN forgets a VPN network and ensure it's forgotten.
// This utility can only work when the detail page of the desired VPN network is opened.
func ForgetVPN(ctx context.Context, tconn *chrome.TestConn, vpnName string) error {
	settings := New(tconn)
	return uiauto.Combine("forget VPN",
		settings.LeftClick(nodewith.Name("Forget").Role(role.Button)),
		settings.WaitUntilExists(nodewith.Name("VPN").Role(role.Heading)),
		settings.WaitUntilGone(nodewith.NameContaining(vpnName)),
	)(ctx)
}

// OpenJoinVPNDialog launches the "Join VPN network" dialog from OS-Settings.
func OpenJoinVPNDialog(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (_ *OSSettings, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	settings, err := Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the OS settings page")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			settings.Close(ctx)
		}
	}(cleanupCtx)

	if err := settings.NavigateToPageURL(ctx, cr, "internet", settings.Exists(Internet)); err != nil {
		return nil, errors.Wrap(err, "failed to open the OS settings page")
	}

	if err := uiauto.Combine(`open the "Join VPN network" dialog`,
		settings.LeftClick(AddConnectionButton),
		settings.LeftClick(nodewith.NameContaining("Add built-in VPN").Role(role.Button)),
	)(ctx); err != nil {
		return nil, err
	}

	return settings, nil
}

// VPNDialogHelper defines a helper for configuring the "Join VPN dialog".
// The "Join VPN dialog" must be opened before utilizing the helper.
type VPNDialogHelper interface {
	// FillInVPNConfigurations fills in the VPN configurations on the "Join VPN dialog".
	FillInVPNConfigurations(context.Context, *chrome.Chrome, *chrome.TestConn, *input.KeyboardEventWriter) error

	// ConnectAndWait clicks the "Connect" button on the "Join VPN dialog" and then navigates to the detail page of the VPN network,
	// waiting for it to be connected.
	// Note that the dialog will be dismissed once the "Connect" button on the dialog is clicked.
	ConnectAndWait(context.Context, *chrome.TestConn) error
}

// NewVPNDialogHelper returns a helper capable of configuring different types of VPN network.
// The |clientCertName| is optional, only required for some VPN types, an error will be thrown if it is not specified when it should be.
func NewVPNDialogHelper(vpnType vpn.Type, props *vpn.ShillProperties, vpnName string, clientCertName *string) (VPNDialogHelper, error) {
	baseHelper := &vpnDialogHelperBase{props: props, vpnName: vpnName}

	switch vpnType {
	case vpn.TypeIKEv2:
		authType, err := props.IPsecAuthType()
		if err != nil {
			return nil, err
		}
		if clientCertName == nil && authType == vpn.AuthTypeCert {
			return nil, errors.Errorf("expecting clientCertName to be provided for %q", vpnType.String())
		}
		return &ikev2Helper{
			vpnDialogHelperBase: baseHelper,
			clientCertName:      *clientCertName,
		}, nil
	case vpn.TypeL2TPIPsec:
		authType, err := props.IPsecAuthType()
		if err != nil {
			return nil, err
		}
		if clientCertName == nil && authType == vpn.AuthTypeCert {
			return nil, errors.Errorf("expecting clientCertName to be provided for %q", vpnType.String())
		}
		return &l2tpipsecHelper{
			vpnDialogHelperBase: baseHelper,
			clientCertName:      *clientCertName,
		}, nil
	case vpn.TypeOpenVPN:
		if clientCertName == nil {
			return nil, errors.Errorf("expecting clientCertName to be provided for %q", vpnType.String())
		}
		return &openvpnHelper{
			vpnDialogHelperBase: baseHelper,
			clientCertName:      *clientCertName,
		}, nil
	case vpn.TypeWireGuard:
		return &wireguardHelper{
			vpnDialogHelperBase: baseHelper,
		}, nil
	default:
		return nil, errors.Errorf("unsupported VPN type: %q", vpnType.String())
	}
}

// NewVPNDialogHelperWithVPNServer returns a helper to configure a specific VPN
// network.
// It starts a VPN server (and necessary network environment) based on the
// specified configs, returns the started VPN server, the helper, and a cleanup
// closure.
// The returned helper will configure a VPN network, connect to the started VPN
// server.
// Caller MUST call the returned cleanup closure to clean up resources.
// Note that vpnNameOnUI is only a name of this VPN network that end user
// attempt to add through ChromeOS UI.
func NewVPNDialogHelperWithVPNServer(ctx context.Context, vpnCfg *vpn.Config, vpnNameOnUI string) (vpnServer *vpn.Server, vpnDialogHelper VPNDialogHelper, cleanup uiauto.Action, retErr error) {
	var cleanups []uiauto.Action

	// This function will start some processes which are supposed to be kept
	// running, so we do not want to use a shortened ctx or a ctx that will be
	// canceled.
	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create network topology for VPN tests")
	}
	defer func() {
		if retErr != nil {
			// Time is not reserved since we do not want to shorten the
			// original ctx.
			if err := networkEnv.TearDown(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to tear down the network topology: ", err)
			}
		}
	}()
	// Insert at front to reverse the cleanup order.
	cleanups = append([]uiauto.Action{networkEnv.TearDown}, cleanups...)

	// This function will start some processes which are supposed to be kept
	// running, so we do not want to use a shortened ctx or a ctx that will be
	// canceled.
	vpnServer, err = vpn.StartServerWithConfig(ctx, networkEnv.Server1, vpnCfg)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to start a VPN server")
	}
	defer func() {
		if retErr != nil {
			// Time is not reserved since we do not want to shorten the
			// original ctx.
			if err := vpnServer.Exit(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to exit the vpn server: ", err)
			}
		}
	}()
	// Insert at front to reverse the cleanup order.
	cleanups = append([]uiauto.Action{vpnServer.Exit}, cleanups...)

	vpnProps, err := vpn.CreateProperties(vpnServer, nil /*secondServer*/)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to generate D-Bus properties")
	}

	certName := fmt.Sprintf("%s [%s]", vpnCfg.CertVals.CACred.Info.CommonName, vpnCfg.CertVals.ClientCred.Info.CommonName)
	if vpnDialogHelper, err = NewVPNDialogHelper(vpnServer.Config.Type, vpnProps, vpnNameOnUI, &certName); err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create a UI helper")
	}

	return vpnServer, vpnDialogHelper, uiauto.Combine("tear down VPN network topology and stop VPN server", cleanups...), nil
}

type vpnDialogHelperBase struct {
	props   *vpn.ShillProperties
	vpnName string
}

func (helper *vpnDialogHelperBase) ConnectAndWait(ctx context.Context, tconn *chrome.TestConn) error {
	settings := New(tconn)
	return uiauto.Combine("connect VPN",
		settings.LeftClick(nodewith.Name("Connect").Role(role.Button)),
		settings.LeftClick(nodewith.Name("VPN").Role(role.Button)),
		settings.LeftClick(nodewith.NameContaining(helper.vpnName+", Details")),
		settings.WaitUntilExists(nodewith.Name(fmt.Sprintf("%s subpage back button", helper.vpnName)).Role(role.Button).HasClass("icon-arrow-back")),
		settings.WaitUntilGone(nodewith.NameStartingWith("Connecting").Role(role.StaticText)),
		settings.WithTimeout(5*time.Second).WaitUntilExists(nodewith.Name("Connected").Role(role.StaticText)),
	)(ctx)
}

type ikev2Helper struct {
	*vpnDialogHelperBase
	clientCertName string
}

func (helper *ikev2Helper) FillInVPNConfigurations(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) error {
	settings := New(tconn)
	comboBox := nodewith.Role(role.ComboBoxSelect).Ancestor(WindowFinder)

	if err := uiauto.Combine("fill in VPN service name and server hostname",
		inputTextField(settings, kb, "Service name", helper.vpnName),
		dropdown.SelectDropDownOption(tconn, comboBox.Name("Provider type"), "IPsec (IKEv2)"),
		inputTextField(settings, kb, "Server hostname", helper.props.GetString("Provider.Host")),
	)(ctx); err != nil {
		return err
	}

	authType, err := helper.props.IPsecAuthType()
	if err != nil {
		return err
	}

	switch authType {
	case vpn.AuthTypeCert:
		return uiauto.Combine("configure VPN",
			dropdown.SelectDropDownOption(tconn, comboBox.Name("Authentication type"), "User certificate"),
			// The server CA is automatically selected by default.
			// Just select the user certificate during the setup.
			dropdown.SelectDropDownOption(tconn, comboBox.Name("User certificate"), helper.clientCertName),
			inputTextField(settings, kb, "Remote identity (optional)", helper.props.GetString("IKEv2.RemoteIdentity")),
		)(ctx)
	case vpn.AuthTypeEAP:
		return uiauto.Combine("configure VPN",
			dropdown.SelectDropDownOption(tconn, comboBox.Name("Authentication type"), "Username and password"),
			inputTextField(settings, kb, "Username", helper.props.GetString("EAP.Identity")),
			inputTextField(settings, kb, "Password", helper.props.GetString("EAP.Password")),
		)(ctx)
	case vpn.AuthTypePSK:
		return uiauto.Combine("configure VPN",
			dropdown.SelectDropDownOption(tconn, comboBox.Name("Authentication type"), "Pre-shared key"),
			inputTextField(settings, kb, "Pre-shared key", helper.props.GetString("IKEv2.PSK")),
			inputTextField(settings, kb, "Local identity (optional)", helper.props.GetString("IKEv2.LocalIdentity")),
			inputTextField(settings, kb, "Remote identity (optional)", helper.props.GetString("IKEv2.RemoteIdentity")),
		)(ctx)
	default:
		return errors.Errorf("unknown auth type %s", authType)
	}
}

type l2tpipsecHelper struct {
	*vpnDialogHelperBase
	clientCertName string
}

func (helper *l2tpipsecHelper) FillInVPNConfigurations(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) error {
	settings := New(tconn)
	comboBox := nodewith.Role(role.ComboBoxSelect).Ancestor(WindowFinder)

	if err := uiauto.Combine("fill in VPN service name and server hostname",
		inputTextField(settings, kb, "Service name", helper.vpnName),
		dropdown.SelectDropDownOption(tconn, comboBox.Name("Provider type"), "L2TP/IPsec"),
		inputTextField(settings, kb, "Server hostname", helper.props.GetString("Provider.Host")),
		inputTextField(settings, kb, "Username", helper.props.GetString("L2TPIPsec.User")),
		inputTextField(settings, kb, "Password", helper.props.GetString("L2TPIPsec.Password")),
	)(ctx); err != nil {
		return err
	}

	authType, err := helper.props.IPsecAuthType()
	if err != nil {
		return err
	}

	switch authType {
	case vpn.AuthTypeCert:
		return uiauto.Combine("configure VPN",
			dropdown.SelectDropDownOption(tconn, comboBox.Name("Authentication type"), "User certificate"),
			// The server CA is automatically selected by default.
			// Just select the user certificate during the setup.
			dropdown.SelectDropDownOption(tconn, comboBox.Name("User certificate"), helper.clientCertName),
		)(ctx)
	case vpn.AuthTypePSK:
		return uiauto.Combine("configure VPN",
			// Authentication type is default to "Pre-shared key".
			inputTextField(settings, kb, "Pre-shared key", helper.props.GetString("L2TPIPsec.PSK")),
		)(ctx)
	default:
		return errors.Errorf("unknown auth type %s", authType)
	}
}

type openvpnHelper struct {
	*vpnDialogHelperBase
	clientCertName string
}

func (helper *openvpnHelper) FillInVPNConfigurations(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) error {
	settings := New(tconn)
	comboBox := nodewith.Role(role.ComboBoxSelect).Ancestor(WindowFinder)

	return uiauto.Combine("configure VPN",
		inputTextField(settings, kb, "Service name", helper.vpnName),
		dropdown.SelectDropDownOption(tconn, comboBox.Name("Provider type"), "OpenVPN"),
		inputTextField(settings, kb, "Server hostname", helper.props.GetString("Provider.Host")),
		inputTextField(settings, kb, "Username", helper.props.GetString("OpenVPN.User")),
		inputTextField(settings, kb, "Password", helper.props.GetString("OpenVPN.Password")),
		// The server CA is automatically selected by default.
		// Just select the user certificate during the setup.
		dropdown.SelectDropDownOption(tconn, comboBox.Name("User certificate"), helper.clientCertName),
	)(ctx)
}

type wireguardHelper struct {
	*vpnDialogHelperBase
}

func (helper *wireguardHelper) FillInVPNConfigurations(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) error {
	settings := New(tconn)
	addrs := helper.props.Get("WireGuard.IPAddress").([]string)

	peers := helper.props.Get("WireGuard.Peers").([]map[string]string)
	// Assume that the first peer is used since this is an invalid configuration for UI input for now.
	// TODO: Update the handling here once the UI supports the configuring of other peers.
	if len(peers) != 1 {
		return errors.Errorf("invalid configuration: expecting only 1 peer, got: %d", len(peers))
	}
	peer := peers[0]

	comboBox := nodewith.Role(role.ComboBoxSelect).Ancestor(WindowFinder)
	return uiauto.Combine("configure VPN",
		inputTextField(settings, kb, "Service name", helper.vpnName),
		dropdown.SelectDropDownOption(tconn, comboBox.Name("Provider type"), "WireGuard"),
		inputTextField(settings, kb, "Client IP address", strings.Join(addrs, ",")),
		dropdown.SelectDropDownOption(tconn, comboBox.Name("Key"), "I have a keypair"),
		inputTextField(settings, kb, "Private key", helper.props.GetString("WireGuard.PrivateKey")),
		inputTextField(settings, kb, "Public key", peer["PublicKey"]),
		inputTextField(settings, kb, "Preshared key", peer["PresharedKey"]),
		inputTextField(settings, kb, "Endpoint", peer["Endpoint"]),
		inputTextField(settings, kb, "Allowed IPs", peer["AllowedIPs"]),
	)(ctx)
}

func inputTextField(settings *OSSettings, kb *input.KeyboardEventWriter, name, value string) action.Action {
	return uiauto.Combine(fmt.Sprintf("input %s", name),
		settings.FocusAndWait(nodewith.Name(name).Role(role.TextField)),
		kb.TypeAction(value),
	)
}
