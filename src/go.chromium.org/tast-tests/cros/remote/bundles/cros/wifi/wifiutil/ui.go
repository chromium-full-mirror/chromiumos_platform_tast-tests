// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"time"

	"go.chromium.org/tast/core/errors"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
)

// JoinWifiServiceNames is the services needed for joining WiFi, through UI operations.
var JoinWifiServiceNames = []string{
	"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
	"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
	"tast.cros.ui.AutomationService",
	"tast.cros.inputs.KeyboardService",
}

var (
	// JoinWiFiNetworkDialogFinder is the dialog opened when connecting to a secured WiFi network.
	JoinWiFiNetworkDialogFinder = ui.Node().NameContaining("Join Wi-Fi network").Role(ui.Role_ROLE_DIALOG).Finder()

	// PasswordFieldFinder is the password text field within the "Join Wi-Fi network" dialog.
	PasswordFieldFinder = ui.Node().NameContaining("Password").Role(ui.Role_ROLE_TEXT_FIELD).Ancestor(JoinWiFiNetworkDialogFinder).Finder()

	// SsidFieldFinder is the SSID text field within the "Join Wi-Fi network" dialog.
	SsidFieldFinder = ui.Node().NameContaining("SSID").Role(ui.Role_ROLE_TEXT_FIELD).Ancestor(JoinWiFiNetworkDialogFinder).Finder()

	// ConnectButtonFinder is the connect button within the "Join Wi-Fi network" dialog.
	ConnectButtonFinder = ui.Node().NameContaining("Connect").Role(ui.Role_ROLE_BUTTON).Ancestor(JoinWiFiNetworkDialogFinder).Finder()
)

// Definitions of the common options in the certificate option combo-box.
const (
	CertificateComboBoxOptionDoNotCheck    = "Do not check"
	CertificateComboBoxOptionDefault       = "Default"
	CertificateComboBoxOptionNoneInstalled = "None installed"
)

// Credential defines the general configuration of a WiFi network in the "Join Wi-Fi" dialog.
type Credential interface {
	// security returns the name of security.
	security() string
}

// None holds the configuration details of the not protected network,
// used for configure the not protected network via "Join Wi-Fi" dialog.
type None struct {
}

func (*None) security() string { return "None" }

// Psk holds the configuration details of the pre-shared key protected network,
// used for configure the WPA/RSN network via "Join Wi-Fi" dialog.
type Psk struct {
	Password string
}

func (*Psk) security() string { return "PSK (WPA or RSN)" }

// EAP defines the authentication information of Extensible Authentication Protocol.
type EAP interface {
	method() string
	caCert() string
	identity() string
	password() string
}

// TLS holds the configuration details of EAP-TLS method,
// used for configure the EAP network via "Join Wi-Fi" dialog.
type TLS struct {
	Identity   string
	CaCert     string
	ClientCert string
}

func (*TLS) security() string   { return "EAP" }
func (*TLS) method() string     { return "EAP-TLS" }
func (e *TLS) identity() string { return e.Identity }
func (*TLS) password() string   { return "" }
func (e *TLS) caCert() string   { return e.CaCert }

// TTLS holds the configuration details of the EAP-TTLS method,
// used for configure the EAP network via "Join Wi-Fi" dialog.
type TTLS struct {
	Password string
	Identity string
	CaCert   string
}

func (*TTLS) security() string   { return "EAP" }
func (*TTLS) method() string     { return "EAP-TTLS" }
func (e *TTLS) identity() string { return e.Identity }
func (e *TTLS) password() string { return e.Password }
func (e *TTLS) caCert() string   { return e.CaCert }

// PEAP holds the configuration details of the EAP-PEAP method,
// used for configure the EAP network via "Join Wi-Fi" dialog.
type PEAP struct {
	Password string
	Identity string
	CaCert   string
}

func (*PEAP) security() string   { return "EAP" }
func (*PEAP) method() string     { return "PEAP" }
func (e *PEAP) identity() string { return e.Identity }
func (e *PEAP) password() string { return e.Password }
func (e *PEAP) caCert() string   { return e.CaCert }

// JoinWifiFromQuickSettings joins to a specified network by opening the "Join Wi-Fi network" dialog
// from the QuickSettings and interacting with it.
//
// Types that are assignable to |cred|
//
//	*None
//	*Psk
//	*TLS
//	*TTLS
//	*PEAP
//
// This method will not leave a clean state, instead, a closure will be returned for cleanup,
// separated cleanup closure allows tests to capture UI tree and screenshot on error.
func JoinWifiFromQuickSettings(ctx context.Context, conn *grpc.ClientConn, ssid string, cred Credential) (func(context.Context), error) {
	cleanup, err := OpenJoinWiFiDialogFromQuickSettings(ctx, conn)
	if err != nil {
		return cleanup, errors.Wrap(err, `failed to open "Join Wi-Fi network" dialog`)
	}

	if err := CompleteJoinWiFiDialog(ctx, conn, ssid, cred, false /*skipSecurityComboBox*/); err != nil {
		return cleanup, errors.Wrap(err, `failed to join Wi-Fi`)
	}

	return cleanup, nil
}

// JoinWifiFromOSSettings joins to a specified network by opening the "Join Wi-Fi network" dialog
// from the OS-Settings and interacting with it.
//
// Types that are assignable to |cred|
//
//	*None
//	*Psk
//	*TLS
//	*TTLS
//	*PEAP
//
// This method will not leave a clean state, instead, a closure will be returned for cleanup,
// separated cleanup closure allows tests to capture UI tree and screenshot on error.
func JoinWifiFromOSSettings(ctx context.Context, conn *grpc.ClientConn, ssid string, cred Credential) (func(context.Context), error) {
	cleanup, err := OpenJoinWiFiDialogFromOSSettings(ctx, conn)
	if err != nil {
		return cleanup, errors.Wrap(err, `failed to open "Join Wi-Fi network" dialog`)
	}

	if err := CompleteJoinWiFiDialog(ctx, conn, ssid, cred, false /*skipSecurityComboBox*/); err != nil {
		return cleanup, errors.Wrap(err, `failed to join Wi-Fi`)
	}

	return cleanup, nil
}

// OpenJoinWiFiDialogFromQuickSettings opens the "Join Wi-Fi network" dialog from the QuickSettings.
//
// This method will not leave a clean state, instead, a closure will be returned for cleanup,
// separated cleanup closure allows tests to capture UI tree and screenshot on error.
func OpenJoinWiFiDialogFromQuickSettings(ctx context.Context, conn *grpc.ClientConn) (func(context.Context), error) {
	quicksettings := quicksettings.NewQuickSettingsServiceClient(conn)
	if _, err := quicksettings.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		return func(ctx context.Context) {}, errors.Wrap(err, "failed to navigate to the detailed Network within QuickSettings")
	}

	uiauto := ui.NewAutomationServiceClient(conn)
	cleanup := func(ctx context.Context) { quicksettings.Hide(ctx, &emptypb.Empty{}) }

	// Ensure WiFi is enabled before joining to a new network.
	wifiClient := wifi.NewShillServiceClient(conn)
	if _, err := wifiClient.SetWifiEnabled(ctx, &wifi.SetWifiEnabledRequest{Enabled: true}); err != nil {
		return cleanup, errors.Wrap(err, "could not enable Wi-Fi using Shill")
	}

	joinWiFiButton := ui.Node().Name("Join Wi-Fi network").Role(ui.Role_ROLE_BUTTON).Finder()
	// DoDefault is a better choice, as the button is at the bottom of the network list,
	// which the list change whenever the available network list change, makes the click action highly unstable.
	if _, err := uiauto.DoDefault(ctx, &ui.DoDefaultRequest{Finder: joinWiFiButton}); err != nil {
		return cleanup, errors.Wrap(err, "failed to click the join button")
	}

	if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: JoinWiFiNetworkDialogFinder}); err != nil {
		return cleanup, errors.Wrap(err, `failed to find the "Join Wi-Fi" dialog`)
	}

	// The QuickSettings should be dismissed once the dialog been opened, replace the cleanup with close-dialog.
	return func(ctx context.Context) {
		exists, err := StableCheckIsNodeFound(ctx, uiauto, JoinWiFiNetworkDialogFinder)
		if err != nil || !exists {
			return
		}
		closeDialogButton := ui.Node().Name("Cancel").Role(ui.Role_ROLE_BUTTON).Ancestor(JoinWiFiNetworkDialogFinder).Finder()
		uiauto.LeftClick(ctx, &ui.LeftClickRequest{Finder: closeDialogButton})
	}, nil
}

// OpenJoinWiFiDialogFromOSSettings opens the "Join Wi-Fi network" dialog from the OS-Settings.
//
// This method will not leave a clean state since the "Join Wi-Fi network" dialog from the OS-Settings
// is a pop-up view inside the OS-Settings, it will be closed with the OS-Settings close.
// Instead, a closure will be returned for cleanup, separated cleanup closure also allows tests to capture UI tree and screenshot on error.
func OpenJoinWiFiDialogFromOSSettings(ctx context.Context, conn *grpc.ClientConn) (func(context.Context), error) {
	settings := ossettings.NewOsSettingsServiceClient(conn)
	if _, err := settings.LaunchAtNetwork(ctx, &emptypb.Empty{}); err != nil {
		return func(ctx context.Context) {}, errors.Wrap(err, `failed to launch OS-Settings at Network page`)
	}

	cleanup := func(ctx context.Context) { settings.Close(ctx, &emptypb.Empty{}) }

	uiauto := ui.NewAutomationServiceClient(conn)
	addConnectionBtn := ui.Node().Name("Add network connection").Role(ui.Role_ROLE_BUTTON).Finder()
	if _, err := uiauto.LeftClick(ctx, &ui.LeftClickRequest{Finder: addConnectionBtn}); err != nil {
		return cleanup, errors.Wrap(err, "failed to click the join button")
	}

	// Ensure WiFi is enabled before joining to a new network.
	wifiClient := wifi.NewShillServiceClient(conn)
	if _, err := wifiClient.SetWifiEnabled(ctx, &wifi.SetWifiEnabledRequest{Enabled: true}); err != nil {
		return cleanup, errors.Wrap(err, "could not enable Wi-Fi using Shill")
	}

	addWifiBtn := ui.Node().Name("Add Wi-Fi…").Role(ui.Role_ROLE_BUTTON).Finder()
	if _, err := uiauto.LeftClick(ctx, &ui.LeftClickRequest{Finder: addWifiBtn}); err != nil {
		return cleanup, errors.Wrap(err, "failed to click the join button")
	}

	if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: JoinWiFiNetworkDialogFinder}); err != nil {
		return cleanup, errors.Wrap(err, `failed to find the "Join Wi-Fi" dialog`)
	}

	return cleanup, nil
}

// CompleteJoinWiFiDialog completes the setups in "Join Wi-Fi network" dialog.
func CompleteJoinWiFiDialog(ctx context.Context, conn *grpc.ClientConn, ssid string, cred Credential, skipSecurityComboBox bool) (retErr error) {
	uiauto := ui.NewAutomationServiceClient(conn)
	if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: JoinWiFiNetworkDialogFinder}); err != nil {
		return errors.Wrap(err, `failed to find the "Join Wi-Fi network" dialog`)
	}

	keyboard := inputs.NewKeyboardServiceClient(conn)

	if err := setTextField(ctx, uiauto, keyboard, SsidFieldFinder, ssid); err != nil {
		return errors.Wrap(err, "failed to set the SSID")
	}

	if err := setCredential(ctx, uiauto, keyboard, cred, skipSecurityComboBox); err != nil {
		return errors.Wrap(err, "failed to set security configuration details")
	}

	if _, err := uiauto.LeftClick(ctx, &ui.LeftClickRequest{Finder: ConnectButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to click connect button")
	}

	return nil
}

func setCredential(ctx context.Context, uiauto ui.AutomationServiceClient, keyboard inputs.KeyboardServiceClient, cred Credential, skipSecurityComboBox bool) error {
	// Early return for an unsecured network.
	if _, ok := cred.(*None); ok {
		return nil
	}

	dialogNodes := ui.Node().Ancestor(JoinWiFiNetworkDialogFinder)
	comboBoxItems := dialogNodes.Role(ui.Role_ROLE_COMBO_BOX_SELECT)

	setEapOptions := func(ctx context.Context, eap EAP) error {
		if _, err := uiauto.SelectDropDownOption(ctx,
			&ui.SelectDropDownOptionRequest{Finder: comboBoxItems.Name("EAP method").Finder(), OptionName: eap.method()}); err != nil {
			return errors.Wrap(err, "failed to click the EAP method combo-box")
		}

		if _, err := uiauto.SelectDropDownOption(ctx,
			&ui.SelectDropDownOptionRequest{Finder: comboBoxItems.Name("Server CA certificate").Finder(), OptionName: eap.caCert()}); err != nil {
			return errors.Wrap(err, "failed to click the CA certificate combo-box")
		}

		identityField := dialogNodes.Name("Identity").Role(ui.Role_ROLE_TEXT_FIELD).Finder()
		if err := setTextField(ctx, uiauto, keyboard, identityField, eap.identity()); err != nil {
			return errors.Wrap(err, "failed to fill the identity")
		}

		return nil
	}

	if !skipSecurityComboBox {
		if _, err := uiauto.SelectDropDownOption(ctx, &ui.SelectDropDownOptionRequest{
			Finder:     comboBoxItems.Name("Security").Finder(),
			OptionName: cred.security(),
		}); err != nil {
			return errors.Wrap(err, "failed to click the security combo-box")
		}
	}

	switch c := cred.(type) {
	case *Psk:
		if err := setTextField(ctx, uiauto, keyboard, PasswordFieldFinder, c.Password); err != nil {
			return errors.Wrap(err, "failed to fill the password")
		}
	case *TLS:
		if err := setEapOptions(ctx, c); err != nil {
			return err
		}
		if _, err := uiauto.SelectDropDownOption(ctx,
			&ui.SelectDropDownOptionRequest{Finder: comboBoxItems.Name("User certificate").Finder(), OptionName: c.ClientCert}); err != nil {
			return errors.Wrap(err, "failed to click the client certificate combo-box")
		}
	case *TTLS:
		if err := setEapOptions(ctx, c); err != nil {
			return err
		}
		if err := setTextField(ctx, uiauto, keyboard, PasswordFieldFinder, c.Password); err != nil {
			return errors.Wrap(err, "failed to fill the password")
		}
	case *PEAP:
		if err := setEapOptions(ctx, c); err != nil {
			return err
		}
		if err := setTextField(ctx, uiauto, keyboard, PasswordFieldFinder, c.Password); err != nil {
			return errors.Wrap(err, "failed to fill the password")
		}
	}

	return nil
}

// StableNodeInfo waits until the given node finder exists and returns its information.
func StableNodeInfo(ctx context.Context, uiSvc ui.AutomationServiceClient, finder *ui.Finder) (*ui.InfoResponse, error) {
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: finder}); err != nil {
		return nil, err
	}

	nodeInfo, err := uiSvc.Info(ctx, &ui.InfoRequest{Finder: finder})
	if err != nil {
		return nil, err
	}
	return nodeInfo, nil
}

// StableCheckIsNodeFound waits for the given node finder and returns if the node is found or not.
func StableCheckIsNodeFound(ctx context.Context, uiSvc ui.AutomationServiceClient, finder *ui.Finder) (bool, error) {
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
		Finder:  finder,
		Timeout: durationpb.New(5 * time.Second),
	}); err != nil {
		// The error might be caused due to finder not exists or other types of error.
		// Do an immediately check again to ensure the finder's existence.
		res, err := uiSvc.IsNodeFound(ctx, &ui.IsNodeFoundRequest{Finder: finder})
		if err != nil {
			return false, errors.Wrap(err, "failed to check if the node exists")
		}
		return res.Found, nil
	}
	return true, nil
}

// EnsureNodeExpanded ensures the given node finder is expanded.
func EnsureNodeExpanded(ctx context.Context, uiSvc ui.AutomationServiceClient, finder *ui.NodeHelper) error {
	expendedNode := finder.Expanded(true).Finder()
	found, err := StableCheckIsNodeFound(ctx, uiSvc, expendedNode)
	if err != nil {
		return errors.Wrap(err, "failed to check if the node exists")
	}

	if !found {
		if _, err := uiSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: finder.Finder()}); err != nil {
			return errors.Wrap(err, "failed to click the node")
		}
		if _, err = uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: expendedNode}); err != nil {
			return errors.Wrap(err, "failed to ensure the node section is expended")
		}
	}

	return nil
}

func setTextField(ctx context.Context, uiauto ui.AutomationServiceClient, keyboard inputs.KeyboardServiceClient, textField *ui.Finder, text string) error {
	if _, err := uiauto.EnsureFocused(ctx, &ui.EnsureFocusedRequest{Finder: textField}); err != nil {
		return errors.Wrap(err, "failed to ensure the text field is focused")
	}

	if _, err := keyboard.Accel(ctx, &inputs.AccelRequest{Key: "ctrl+A"}); err != nil {
		return errors.Wrap(err, "failed to select all")
	}

	if _, err := keyboard.Accel(ctx, &inputs.AccelRequest{Key: "backspace"}); err != nil {
		return errors.Wrap(err, "failed to select all")
	}

	if _, err := keyboard.Type(ctx, &inputs.TypeRequest{Key: text}); err != nil {
		return errors.Wrapf(err, "failed to type text %q", text)
	}

	return nil
}

// ConfigureWifiNetwork fills out the network configuration dialog for a secure WiFi network.
// This method expects the network configuration dialog to already be open before being called.
func ConfigureWifiNetwork(ctx context.Context, uiautomation ui.AutomationServiceClient, conn *grpc.ClientConn, passphrase, connectButtonName string) error {
	passwordFieldFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: "Password"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_TEXT_FIELD}},
		},
	}

	if _, err := uiautomation.LeftClick(
		ctx, &ui.LeftClickRequest{Finder: passwordFieldFinder}); err != nil {
		return errors.Wrap(err, "failed to click password field")
	}

	kb := inputs.NewKeyboardServiceClient(conn)

	if _, err := kb.Type(ctx, &inputs.TypeRequest{
		Key: passphrase,
	}); err != nil {
		return errors.Wrap(err, "failed to enter the password for the Wi-Fi network")
	}

	connectButtonFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: connectButtonName}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
		},
	}

	if _, err := uiautomation.LeftClick(
		ctx, &ui.LeftClickRequest{Finder: connectButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to click the connect button")
	}
	return nil
}
