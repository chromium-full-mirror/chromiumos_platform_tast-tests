// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This package provides a set of functions to collect and setup proxy settings
// from both login screen and Settings app.
// This package only works for "Manual Settings Configuration" at the moment.

package proxysettings

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/checked"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/chrome/uiauto/restriction"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/network/netconfig"
	"chromiumos/tast/testing"
)

// Protocol represents the type of proxy protocols.
type Protocol int

const (
	// HTTP represents the HTTP proxy protocol.
	HTTP Protocol = iota
	// HTTPS represents the HTTPS proxy protocol.
	HTTPS
	// Socks represents the SOCKS proxy protocol.
	Socks
	// SameProxy represents all protocols sharing the same proxy values.
	SameProxy
)

// Name returns the name of proxy protocol.
func (p Protocol) Name() string {
	return []string{"HTTP", "HTTPS", "Socks", "SameProxy"}[p]
}

// Config represents the proxy configuration.
type Config struct {
	// Protocol is the type of proxy protocol.
	Protocol Protocol
	// Host is the proxy host.
	Host string
	// Port is the proxy port.
	Port string
}

// HostNode returns the node for the proxy host.
func (c *Config) HostNode() *nodewith.Finder {
	switch c.Protocol {
	case HTTP:
		return ossettings.HTTPHostTextField
	case HTTPS:
		return ossettings.HTTPSHostTextField
	case Socks:
		return ossettings.SocksHostTextField
	case SameProxy:
		return ossettings.SameProxyHostTextField
	default:
		return nil
	}
}

// HostName returns the name of the proxy host.
func (c *Config) HostName() string {
	switch c.Protocol {
	case HTTP:
		return "http host"
	case HTTPS:
		return "https host"
	case Socks:
		return "socks host"
	case SameProxy:
		return "proxy host"
	default:
		return ""
	}
}

// PortNode returns the node for the proxy port.
func (c *Config) PortNode() *nodewith.Finder {
	switch c.Protocol {
	case HTTP:
		return ossettings.HTTPPortTextField
	case HTTPS:
		return ossettings.HTTPSPortTextField
	case Socks:
		return ossettings.SocksPortTextField
	case SameProxy:
		return ossettings.SameProxyPortTextField
	default:
		return nil
	}
}

// PortName returns the name of the proxy port.
func (c *Config) PortName() string {
	switch c.Protocol {
	case HTTP:
		return "http port"
	case HTTPS:
		return "https port"
	case Socks:
		return "socks port"
	case SameProxy:
		return "proxy port"
	default:
		return ""
	}
}

// ConnectionType represents the connection type of proxy settings.
type ConnectionType string

const (
	// DirectInternetConnection is the proxy connect type of direct internet connection.
	DirectInternetConnection ConnectionType = "Direct Internet connection"
	// ManualProxyConfiguration is the proxy connect type of manual proxy configuration.
	ManualProxyConfiguration ConnectionType = "Manual proxy configuration"
)

// ProxySettings represents the proxy-setting page.
// The page could be within the OSSettings window or within the dialog/window on sign-in screen.
// Use 'CollectEthernet' for ethernet or 'CollectWifi' for a specified WiFi when DUT is logged in already.
// Otherwise, use 'CollectEthernetFromSignInScreen' or 'CollectWifiFromSignInScreen' instead.
// The caller is responsible for calling `Close` to close the OSSettings window or within the dialog/window on sign-in screen.
type ProxySettings struct {
	isLoggedIn bool
}

// CollectEthernet launches the network settings.
// |isLoggedIn| should be true while DUT is logged in
// TODO(b/244330490): Update this method to open the network settings by
// clicking the network in the network list in the Quick Settings.
func CollectEthernet(ctx context.Context, tconn *chrome.TestConn, isLoggedIn bool) (*ProxySettings, error) {
	return collectFromQuickSettings(ctx, tconn, netconfig.Ethernet, "", isLoggedIn)
}

// CollectWifi launches the network settings for a particular WiFi network.
// Launch network detail page from OS settings when DUT is logged in. Otherwise, from quick settings.
// The network must be a remembered or opened.
func CollectWifi(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, wifiSsid string, isLoggedIn bool) (*ProxySettings, error) {
	if isLoggedIn {
		if _, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, wifiSsid, netconfig.WiFi); err != nil {
			return nil, errors.Wrap(err, "failed to open specific wifi setting")
		}

		return &ProxySettings{isLoggedIn: true}, expandProxyOption(ctx, tconn)
	}
	return collectFromQuickSettings(ctx, tconn, netconfig.WiFi, wifiSsid, isLoggedIn)
}

// Close clears ProxySettings object and closes Settings app if applied.
func (ps *ProxySettings) Close(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) {
	if ps.isLoggedIn {
		if err := apps.Close(ctx, tconn, apps.Settings.ID); err != nil {
			testing.ContextLog(ctx, "Failed to close Settings app: ", err)
		}
	} else {
		if err := kb.AccelAction("esc")(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close Settings window: ", err)
		}
	}
}

// collectFromQuickSettings launches the proxy setting page of the specified network.
// Note that the network has to be connected to further collect the proxy settings.
func collectFromQuickSettings(ctx context.Context, tconn *chrome.TestConn, networkType netconfig.NetworkType, wifiSsid string, isLoggedIn bool) (*ProxySettings, error) {
	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to navigate to network detailed view")
	}

	var networkList *nodewith.Finder
	switch networkType {
	case netconfig.Ethernet:
		networkList = quicksettings.NetworkListItemView.NameContaining("Ethernet")
	case netconfig.WiFi:
		networkList = quicksettings.NetworkListItemView.NameContaining(wifiSsid)
	default:
		return nil, errors.Errorf("unsupported network type: %d", networkType)
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("open the target network proxy settings page",
		ui.WaitUntilExists(nodewith.NameStartingWith("Connected").Role(role.StaticText).Ancestor(networkList)), // The target network has to be connected.
		ui.LeftClick(networkList),
	)(ctx); err != nil {
		return nil, err
	}

	if isLoggedIn {
		if err := expandProxyOption(ctx, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to expand proxy option on settings")
		}
	}

	return &ProxySettings{isLoggedIn: isLoggedIn}, nil
}

// expandProxyOption expands the proxy option within the OS-Settings.
func expandProxyOption(ctx context.Context, tconn *chrome.TestConn) error {
	app := ossettings.New(tconn)
	if err := app.WaitUntilExists(ossettings.ShowProxySettingsTab)(ctx); err != nil {
		return errors.Wrap(err, "failed to find 'Shared networks' toggle button")
	}

	if err := uiauto.Combine("expand 'Proxy' section",
		app.LeftClick(ossettings.ShowProxySettingsTab),
		app.WaitForLocation(ossettings.SharedNetworksToggleButton),
	)(ctx); err != nil {
		return err
	}

	if toggleInfo, err := app.Info(ctx, ossettings.SharedNetworksToggleButton); err != nil {
		return errors.Wrap(err, "failed to get toggle button info")
	} else if toggleInfo.Checked == checked.True {
		testing.ContextLog(ctx, "'Allow proxies for shared networks' is already turned on")
		return nil
	}

	return uiauto.Combine("turn on 'Allow proxies for shared networks' option",
		app.LeftClick(ossettings.SharedNetworksToggleButton),
		app.LeftClick(ossettings.ConfirmButton),
	)(ctx)
}

// SetManualConfig sets up manual proxy values.
// This function is safe to call with both the Network dialog during sign-in and
// the detailed Network page within the OS Settings.
// To do this, node ancestors are for better flexibility.
func (ps *ProxySettings) SetManualConfig(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, configs []*Config) error {
	ui := uiauto.New(tconn)
	if err := setConnectionType(ctx, ui, ManualProxyConfiguration); err != nil {
		return err
	}

	// The SameHost/SamePort is one and only proxy when using the same proxy.
	useSameProxy := (len(configs) == 1) && (configs[0].Protocol == SameProxy)

	sameProtocolToggle := nodewith.Name("Use the same proxy for all protocols").Role(role.ToggleButton)
	if err := ui.WaitUntilExists(sameProtocolToggle)(ctx); err != nil {
		return errors.Wrap(err, `failed to check "Use the same proxy for all protocols" is enabled or not`)
	}

	if err := uiauto.IfFailThen(
		ui.WaitUntilCheckedState(sameProtocolToggle, useSameProxy),
		ui.WithTimeout(30*time.Second).LeftClickUntil(
			sameProtocolToggle,
			ui.WithTimeout(5*time.Second).WaitUntilCheckedState(sameProtocolToggle, useSameProxy),
		),
	)(ctx); err != nil {
		return errors.Wrap(err, `failed to disable "Use the same proxy for all protocols"`)
	}

	for _, config := range configs {
		if err := uiauto.Combine(fmt.Sprintf("setup proxy, host: %q, port: %q", config.Host, config.Port),
			ui.EnsureFocused(config.HostNode()),
			kb.AccelAction("Ctrl+A"),
			// Clear the content because the host could be blank. When the host is blank
			// this will result in no keys being pressed, and thus the existing content
			// will not be cleared.
			kb.AccelAction("Backspace"),
			kb.TypeAction(config.Host),
			ui.EnsureFocused(config.PortNode()),
			kb.AccelAction("Ctrl+A"),
			// Clear the content because the port could be blank. When the port is blank
			// this will result in no keys being pressed, and thus the existing content
			// will not be cleared.
			kb.AccelAction("Backspace"),
			kb.TypeAction(config.Port),
		)(ctx); err != nil {
			return err
		}
	}

	// The "Use the same proxy for all protocols" toggle button could be (depends on the proxy value) automatically turned on once the values are saved, checking it again before saving it.
	if err := ui.WaitUntilCheckedState(sameProtocolToggle, useSameProxy)(ctx); err != nil {
		return errors.Wrap(err, "failed to check node state")
	}

	saveButton := ossettings.WindowFinder.HasClass("action-button").Name("Save").Role(role.Button)
	return uiauto.Combine("save proxy settings",
		ui.MakeVisible(saveButton),
		ui.WaitForLocation(saveButton),
		// Ensure "Save" button has been clicked and become not clickable.
		ui.WithInterval(time.Second).LeftClickUntil(saveButton, ui.CheckRestriction(saveButton, restriction.Disabled)),
	)(ctx)
}

// SetDirectConnection sets proxy connection type as 'Direct Internet Connection'.
func (ps *ProxySettings) SetDirectConnection(ctx context.Context, ui *uiauto.Context) error {
	return setConnectionType(ctx, ui, DirectInternetConnection)
}

// ManualConfigContent returns the proxy values with specified protocol.
// This function is safe to call when network setup page is launched on
// both OS Settings or on-screen dialog when in login screen. To do this,
// node ancestors are for better flexibility.
func (ps *ProxySettings) ManualConfigContent(ctx context.Context, tconn *chrome.TestConn, protocol Protocol) (*Config, error) {
	proxy := &Config{Protocol: protocol}

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(ossettings.ProxyDropDownMenu)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait until drop down menu exists")
	}

	dropDownMenu, err := ui.Info(ctx, ossettings.ProxyDropDownMenu)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the value of proxy drop down menu")
	}

	// Proxy settings is available only when the connection type is 'Manual proxy configuration'.
	if dropDownMenu.Value != string(ManualProxyConfiguration) {
		return nil, errors.Errorf("unexpected proxy connection type, got: %q, want: %q", dropDownMenu.Value, ManualProxyConfiguration)
	}

	if err := ui.WaitUntilExists(proxy.HostNode())(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to ensure node %q exists and is shown on the screen", proxy.HostName())
	}

	infoHostNode, err := ui.Info(ctx, proxy.HostNode())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get node info for field %q", proxy.HostName())
	}
	proxy.Host = infoHostNode.Value

	if err := ui.WaitUntilExists(proxy.PortNode())(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to ensure node %q exists and is shown on the screen", proxy.PortName())
	}

	infoPortNode, err := ui.Info(ctx, proxy.PortNode())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get node info for field %q", proxy.PortName())
	}
	proxy.Port = infoPortNode.Value

	return proxy, nil
}

// setConnectionType sets proxy connection type to expected type.
func setConnectionType(ctx context.Context, ui *uiauto.Context, connectionType ConnectionType) error {
	option := nodewith.Name(string(connectionType)).Role(role.ListBoxOption)
	return uiauto.Combine(fmt.Sprintf("setup proxy to %q", connectionType),
		ui.LeftClickUntil(ossettings.ProxyDropDownMenu, ui.WithTimeout(3*time.Second).WaitUntilExists(option)),
		ui.LeftClick(option),
		ui.WaitUntilGone(option),
	)(ctx)
}

// IsUseSameProxyToggleOptionEnabled checks whether the toggle option 'Use the same proxy for all protocols' is enabled or not.
func (ps *ProxySettings) IsUseSameProxyToggleOptionEnabled(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	ui := uiauto.New(tconn)

	useSameProxyToggle := nodewith.Name("Use the same proxy for all protocols").Role(role.ToggleButton)
	if err := ui.WaitForLocation(useSameProxyToggle)(ctx); err != nil {
		return false, errors.Wrap(err, "failed to wait until node stable")
	}

	info, err := ui.Info(ctx, useSameProxyToggle)
	if err != nil {
		return false, errors.Wrap(err, "failed to get node info")
	}
	return info.Checked == checked.True, nil
}
