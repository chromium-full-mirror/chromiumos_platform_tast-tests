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

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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

func (p Protocol) hostFieldName() string {
	return []string{"http host", "https host", "socks host", "proxy host"}[p]
}
func (p Protocol) portFieldName() string {
	return []string{"http port", "https port", "socks port", "proxy port"}[p]
}

func (p Protocol) hostFieldNodeFinder() *nodewith.Finder {
	return []*nodewith.Finder{
		ossettings.HTTPHostTextField,
		ossettings.HTTPSHostTextField,
		ossettings.SocksHostTextField,
		ossettings.SameProxyHostTextField,
	}[p]
}

func (p Protocol) portFieldNodeFinder() *nodewith.Finder {
	return []*nodewith.Finder{
		ossettings.HTTPPortTextField,
		ossettings.HTTPSPortTextField,
		ossettings.SocksPortTextField,
		ossettings.SameProxyPortTextField,
	}[p]
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

// ConnectionType represents the connection type of proxy settings.
type ConnectionType string

const (
	// DirectInternetConnection is the proxy connect type of direct internet connection.
	DirectInternetConnection ConnectionType = "Direct Internet connection"
	// ManualProxyConfiguration is the proxy connect type of manual proxy configuration.
	ManualProxyConfiguration ConnectionType = "Manual proxy configuration"
)

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
			settings.MakeVisible(ossettings.ShowProxySettingsButton),
			settings.WaitUntilExists(ossettings.ShowProxySettingsButton.Visible()),
			settings.LeftClickUntil(
				// Expand 'Proxy' section.
				ossettings.ShowProxySettingsButton,
				settings.WithTimeout(5*time.Second).WaitUntilExists(ossettings.ShowProxySettingsButton.Expanded()),
			),
			settings.MakeVisible(ossettings.ProxyDropDownMenu),
			settings.WaitUntilExists(ossettings.ProxyDropDownMenu.Visible()),
			// Wait for the Proxy section to be stable.
			settings.WithTimeout(5*time.Second).WaitForLocation(ossettings.ProxyDropDownMenu),
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

// TargetNetwork describes a network to set the proxy configuration.
type TargetNetwork interface {
	Name() string
	Type() netconfigtypes.NetworkType
}

type ethernet struct{}

// Ethernet returns the Ethernet network.
func Ethernet() TargetNetwork                        { return &ethernet{} }
func (e *ethernet) Name() string                     { return "Ethernet" }
func (e *ethernet) Type() netconfigtypes.NetworkType { return netconfigtypes.Ethernet }

type wifi struct{ ssid string }

// Wifi returns the WiFi network with the specified SSID.
func Wifi(ssid string) TargetNetwork             { return &wifi{ssid: ssid} }
func (w *wifi) Name() string                     { return w.ssid }
func (w *wifi) Type() netconfigtypes.NetworkType { return netconfigtypes.WiFi }

type vpn struct{ name string }

// VPN returns the VPN network with the specified name.
func VPN(name string) TargetNetwork             { return &vpn{name: name} }
func (v *vpn) Name() string                     { return v.name }
func (v *vpn) Type() netconfigtypes.NetworkType { return netconfigtypes.VPN }

// LoginMode defines the login mode of the DUT.
type LoginMode int

// The supported login mode.
const (
	LoggedIn LoginMode = iota
	OOBE
	SignInScreen
)

// Manager manages all interaction with the proxy-settings page.
// Use 'NewProxySettingsManager' to acquire an instance, 'Launch' to open the proxy-settings page and 'Close' to close the page.
//
//	manager := NewProxySettingsManager(LoggedIn)
//	manager.Launch(ctx, cr, tconn, Ethernet())
//	defer manager.Close(ctx)
type Manager struct {
	dialogConn *chrome.Conn
	loginState LoginMode
}

// NewProxySettingsManager returns an instance of ProxySettingsManager.
// |loginState| specifies the login mode of the DUT, which is very essential since different DUT state
// requires different indicator for determinate that the DUT is ready for testing.
func NewProxySettingsManager(loginState LoginMode) *Manager {
	return &Manager{
		loginState: loginState,
	}
}

// LaunchAndPrepare opens the proxy-settings page and prepares the page to be ready for setup proxy.
// Caller is responsible for calling 'Close' once the proxy-settings page is launched.
func (m *Manager) LaunchAndPrepare(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, network TargetNetwork) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := m.Launch(ctx, cr, tconn, network); err != nil {
		return nil
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			m.Close(ctx)
		}
	}(cleanupCtx)

	switch m.loginState {
	case LoggedIn:
		if err := AllowProxiesForSharedNetwork(ctx, tconn, true /* allow */); err != nil {
			return errors.Wrap(err, "failed to expand proxy option on settings")
		}
	}
	return nil
}

// Launch opens the proxy-settings page.
// Caller is responsible for calling 'Close' once the proxy-settings page is launched.
func (m *Manager) Launch(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, network TargetNetwork) error {
	switch m.loginState {
	case OOBE:
		// Launch the proxy-settings page from QuickSettings is unstable as the QuickSettings
		// can be collapsed by various events, wait for the OOBE to be stable is essential.
		oobeConn, err := cr.WaitForOOBEConnection(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to wait for OOBE to be ready for testing")
		}
		defer oobeConn.Close()

		// Further wait for the welcome screen is visible to continue on launch the proxy-settings page.
		if err := oobe.IsWelcomeScreenVisible(ctx, oobeConn); err != nil {
			return errors.Wrap(err, "failed to wait for welcome screen to be visible")
		}

		if err := m.launchProxySettingsFromQuickSettings(ctx, cr, tconn, network); err != nil {
			return errors.Wrap(err, "failed to launch proxy settings from QuickSettings")
		}
	case SignInScreen:
		// Launch the proxy-settings page from QuickSettings is unstable as the QuickSettings
		// can be collapsed by various events, wait for the lock-screen to be stable is essential.
		if err := lockscreen.WaitForPasswordEntry(ctx, tconn, 30*time.Second); err != nil {
			return errors.Wrap(err, "failed to wait for lockscreen to be ready for testing")
		}

		if err := m.launchProxySettingsFromQuickSettings(ctx, cr, tconn, network); err != nil {
			return errors.Wrap(err, "failed to launch proxy settings from QuickSettings")
		}
	case LoggedIn:
		// Launch the proxy-settings page from QuickSettings is unstable.
		// Directly open the detail page of the network from OSSettings since OSSettings is available when device is logged in.
		if err := m.launchProxySettingsFromOSSettings(ctx, cr, tconn, network); err != nil {
			return errors.Wrap(err, "failed to launch proxy settings from OSSettings")
		}

		// The proxy-settings section will be collapsed when device is logged in.
		if err := ExpandProxySettingsSection(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to expand proxy option on settings")
		}
	default:
		return errors.Errorf("unrecognized network %+v", network)
	}

	return nil
}

// Close closes the proxy-settings page.
func (m *Manager) Close(ctx context.Context) error {
	if m.dialogConn != nil {
		if err := m.dialogConn.CloseTarget(ctx); err != nil {
			return errors.Wrap(err, "failed to close proxy settings dialog")
		}
		if err := m.dialogConn.Close(); err != nil {
			return errors.Wrap(err, "failed to close proxy settings dialog connection")
		}
		m.dialogConn = nil
	}
	return nil
}

// launchProxySettingsFromQuickSettings launches the proxy settings dialog for a specified network from the QuickSettings.
func (m *Manager) launchProxySettingsFromQuickSettings(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, network TargetNetwork) (retErr error) {
	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to navigate to network detailed view")
	}
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(ctx, func() bool { return retErr != nil }, tconn, "launch_proxy_settings")

	ui := uiauto.New(tconn)

	networkListItemView, err := quicksettings.NetworkListItemView(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get network list item view")
	}
	networkItem := networkListItemView.NameContaining(network.Name()).First()

	return uiauto.Combine("open the target network proxy settings page",
		ui.WaitUntilExists(nodewith.NameStartingWith("Connected").Role(role.StaticText).Ancestor(networkItem)), // The target network has to be connected.
		ui.LeftClick(networkItem),
		m.waitForProxySettingsOpened(cr, tconn),
	)(ctx)
}

// launchProxySettingsFromOSSettings launches the proxy settings dialog for a specified network from the OSSettings.
func (m *Manager) launchProxySettingsFromOSSettings(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, network TargetNetwork) (retErr error) {
	if _, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, network.Name(), network.Type()); err != nil {
		return errors.Wrap(err, "failed to open network detail page")
	}

	return m.waitForProxySettingsOpened(cr, tconn)(ctx)
}

func (m *Manager) waitForProxySettingsOpened(cr *chrome.Chrome, tconn *chrome.TestConn) action.Action {
	var connectToPage func(ctx context.Context) (*chrome.Conn, error)
	switch m.loginState {
	case LoggedIn:
		connectToPage = func(ctx context.Context) (*chrome.Conn, error) {
			return ossettings.New(tconn).ChromeConn(ctx, cr)
		}
	case OOBE, SignInScreen:
		connectToPage = func(ctx context.Context) (*chrome.Conn, error) {
			return cr.NewConnForTarget(ctx, chrome.MatchTargetURL("chrome://internet-detail-dialog/"))
		}
	default:
		return func(ctx context.Context) error {
			return errors.New("unexpected login state")
		}
	}

	return func(ctx context.Context) error {
		waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
		defer cancelWait()

		conn, err := connectToPage(waitCtx)
		if err != nil {
			return errors.Wrap(err, "failed to find proxy settings dialog")
		}

		// The page needs a moment to be stable.
		// For more information see b/306068737.
		if err := webutil.WaitForQuiescence(ctx, conn, 15*time.Second); err != nil {
			return errors.Wrap(err, "failed to wait for Proxy settings to be stable")
		}

		if m.dialogConn != nil {
			m.dialogConn.Close()
		}
		m.dialogConn = conn

		return nil
	}
}

// SetManualConfig sets up manual proxy values.
// This function is safe to call with both the Network dialog during sign-in and
// the detailed Network page within the OS Settings.
// To do this, node ancestors are for better flexibility.
func (m *Manager) SetManualConfig(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, configs []*Config) error {
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
			ui.EnsureFocused(config.Protocol.hostFieldNodeFinder()),
			kb.AccelAction("Ctrl+A"),
			// Clear the content because the host could be blank. When the host is blank
			// this will result in no keys being pressed, and thus the existing content
			// will not be cleared.
			kb.AccelAction("Backspace"),
			kb.TypeAction(config.Host),
			ui.EnsureFocused(config.Protocol.portFieldNodeFinder()),
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
func (m *Manager) SetDirectConnection(ctx context.Context, ui *uiauto.Context) error {
	return setConnectionType(ctx, ui, DirectInternetConnection)
}

// ManualConfigContent returns the proxy values with specified protocol.
// This function is safe to call when network setup page is launched on
// both OS Settings or on-screen dialog when in login screen. To do this,
// node ancestors are for better flexibility.
func (m *Manager) ManualConfigContent(ctx context.Context, tconn *chrome.TestConn, protocol Protocol) (*Config, error) {
	proxy := &Config{Protocol: protocol}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("wait for drop down menu and make it visible",
		ui.WaitUntilExists(ossettings.ProxyDropDownMenu),
		ui.MakeVisible(ossettings.ProxyDropDownMenu),
		ui.WaitUntilExists(ossettings.ProxyDropDownMenu.Visible()),
	)(ctx); err != nil {
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

	if err := ui.WaitUntilExists(proxy.Protocol.hostFieldNodeFinder())(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to ensure node %q exists and is shown on the screen", proxy.Protocol.hostFieldName())
	}

	infoHostNode, err := ui.Info(ctx, proxy.Protocol.hostFieldNodeFinder())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get node info for field %q", proxy.Protocol.hostFieldName())
	}
	proxy.Host = infoHostNode.Value

	if err := ui.WaitUntilExists(proxy.Protocol.portFieldNodeFinder())(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to ensure node %q exists and is shown on the screen", proxy.Protocol.portFieldName())
	}

	infoPortNode, err := ui.Info(ctx, proxy.Protocol.portFieldNodeFinder())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get node info for field %q", proxy.Protocol.portFieldName())
	}
	proxy.Port = infoPortNode.Value

	return proxy, nil
}

// IsUseSameProxyToggleOptionEnabled checks whether the toggle option 'Use the same proxy for all protocols' is enabled or not.
func (m *Manager) IsUseSameProxyToggleOptionEnabled(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
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

// setConnectionType sets proxy connection type to expected type.
func setConnectionType(ctx context.Context, ui *uiauto.Context, connectionType ConnectionType) error {
	option := nodewith.Name(string(connectionType)).Role(role.ListBoxOption)
	return uiauto.Combine(fmt.Sprintf("setup proxy to %q", connectionType),
		ui.MakeVisible(ossettings.ProxyDropDownMenu),
		ui.WaitUntilExists(ossettings.ProxyDropDownMenu.Visible()),
		ui.LeftClickUntil(ossettings.ProxyDropDownMenu, ui.WithTimeout(3*time.Second).WaitUntilExists(option)),
		ui.LeftClick(option),
		ui.WaitUntilGone(option),
	)(ctx)
}
