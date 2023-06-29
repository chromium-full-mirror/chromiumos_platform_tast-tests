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

	"go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
	return collectFromQuickSettings(ctx, tconn, netconfigtypes.Ethernet, "", isLoggedIn)
}

// CollectWifi launches the network settings for a particular WiFi network.
// Launch network detail page from OS settings when DUT is logged in. Otherwise, from quick settings.
// The network must be a remembered or opened.
func CollectWifi(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, wifiSsid string, isLoggedIn bool) (*ProxySettings, error) {
	if isLoggedIn {
		return collectFromOsSettings(ctx, cr, tconn, wifiSsid)
	}
	return collectFromQuickSettings(ctx, tconn, netconfigtypes.WiFi, wifiSsid, isLoggedIn)
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

// launchProxySettingsFromQuickSettings launches the proxy settings dialog for a specified network from quick settings.
func launchProxySettingsFromQuickSettings(ctx context.Context, tconn *chrome.TestConn, networkType netconfigtypes.NetworkType, wifiSsid string) error {
	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to navigate to network detailed view")
	}
	networkListItemView, err := quicksettings.NetworkListItemView(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get network list item view")
	}
	var networkList *nodewith.Finder
	switch networkType {
	case netconfigtypes.Ethernet:
		networkList = networkListItemView.NameContaining("Ethernet")
	case netconfigtypes.WiFi:
		networkList = networkListItemView.NameContaining(wifiSsid)
	default:
		return errors.Errorf("unsupported network type: %d", networkType)
	}

	ui := uiauto.New(tconn)
	return uiauto.Combine("open the target network proxy settings page",
		ui.WaitUntilExists(nodewith.NameStartingWith("Connected").Role(role.StaticText).Ancestor(networkList)), // The target network has to be connected.
		ui.LeftClick(networkList),
	)(ctx)
}

// collectFromQuickSettings launches the proxy setting page of the specified network.
// Note that the network has to be connected to further collect the proxy settings.
func collectFromQuickSettings(ctx context.Context, tconn *chrome.TestConn, networkType netconfigtypes.NetworkType, wifiSsid string, isLoggedIn bool) (*ProxySettings, error) {
	if err := launchProxySettingsFromQuickSettings(ctx, tconn, networkType, wifiSsid); err != nil {
		return nil, errors.Wrap(err, "failed to launch proxy settings from QuickSettings")
	}

	if isLoggedIn {
		if err := prepareProxySettingsSection(ctx, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to prepare proxy settings section")
		}
	}

	return &ProxySettings{isLoggedIn: isLoggedIn}, nil
}

// launchProxySettingsFromOsSettings launches the proxy settings page for a specified network from os-settings.
func launchProxySettingsFromOsSettings(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, wifiSsid string) error {
	_, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, wifiSsid, netconfigtypes.WiFi)
	return err
}

// collectFromOsSettings launches the proxy settings page for a specified network from os-settings, expand the proxy sections and
// turn on the 'Allow proxies for shared network' toggle button.
func collectFromOsSettings(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, wifiSsid string) (*ProxySettings, error) {
	if err := launchProxySettingsFromOsSettings(ctx, cr, tconn, wifiSsid); err != nil {
		return nil, errors.Wrap(err, "failed to launch proxy settings from QuickSettings")
	}

	if err := prepareProxySettingsSection(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to prepare proxy settings section")
	}

	// OSSettings can only be launched after DUT is logged in.
	return &ProxySettings{isLoggedIn: true}, nil
}

// prepareProxySettingsSection prepare the proxy settings section to be able to setup proxies.
func prepareProxySettingsSection(ctx context.Context, tconn *chrome.TestConn) error {
	if err := ExpandProxySettingsSection(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to expand proxy option on settings")
	}
	if err := AllowProxiesForSharedNetwork(ctx, tconn, true /* allow */); err != nil {
		return errors.Wrap(err, "failed to allows or disallows proxies for shared networks")
	}
	return nil
}

// ExpandProxySettingsSection ensures the proxy settings section of a network to be expanded.
// This method should only be called from the network detail page of a network within OS Settings.
// Calling on the WebUI before login fails since the proxy settings are not within an expandable section.
func ExpandProxySettingsSection(ctx context.Context, tconn *chrome.TestConn) error {
	// This method should only be called from the network detail page of a network within OS Settings,
	// so all nodes should be scoped under the OS-Settings.
	settings := ossettings.New(tconn)

	if err := settings.WaitUntilExists(ossettings.ShowProxySettingsButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the 'Show proxy settings' button")
	}

	return uiauto.IfSuccessThen(
		settings.WaitUntilExists(ossettings.ShowProxySettingsButton.Collapsed()),
		uiauto.Combine("expand 'Proxy' section",
			settings.LeftClick(ossettings.ShowProxySettingsButton),
			settings.WaitForLocation(ossettings.ProxyDropDownMenu), // Wait for the Proxy section is expanded.
		),
	)(ctx)
}

// AllowProxiesForSharedNetwork allows or disallows proxies for shared
// networks by toggling the "Allow proxies for shared networks" toggle button.
// This method should only be called from the network detail page of a network within OS Settings
// that has an expanded proxy settings section.
// Calling on the WebUI before login does nothing since the "Allow proxies for shared networks" toggle button isn't available.
func AllowProxiesForSharedNetwork(ctx context.Context, tconn *chrome.TestConn, allow bool) error {
	// This method should only be called from the network detail page of a network within OS Settings,
	// so all nodes should be scoped under the OS-Settings.
	settings := ossettings.New(tconn)
	expected := checked.True
	if !allow {
		expected = checked.False
	}

	// There is no 'Allow proxies for shared networks' toggle button if the Wi-Fi network is not shared.
	if err := settings.WithTimeout(5 * time.Second).WaitUntilExists(ossettings.SharedNetworksToggleButton)(ctx); err != nil {
		if nodewith.IsNodeNotFoundErr(err) {
			return nil
		}
		return err
	}

	if toggleInfo, err := settings.Info(ctx, ossettings.SharedNetworksToggleButton); err != nil {
		return errors.Wrap(err, "failed to get toggle button info")
	} else if toggleInfo.Checked == expected {
		return nil
	}

	return uiauto.Combine("toggle 'Allow proxies for shared networks' button",
		settings.MakeVisible(ossettings.SharedNetworksToggleButton),
		settings.LeftClick(ossettings.SharedNetworksToggleButton),
		settings.LeftClick(ossettings.ConfirmButton),
		settings.WaitUntilCheckedState(ossettings.SharedNetworksToggleButton, allow),
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
