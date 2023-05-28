// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/proxysettings"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast-tests/cros/services/cros/network"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			network.RegisterProxySettingServiceServer(srv, &ProxySettingsService{
				serviceState: s,
				sharedObject: common.SharedObjectsForServiceSingleton,
			})
		},
	})
}

// ProxySettingsService implements tast.cros.network.ProxySettingsService.
type ProxySettingsService struct {
	sharedObject  *common.SharedObjectsForService
	serviceState  *testing.ServiceState
	kb            *input.KeyboardEventWriter
	proxySettings *proxysettings.ProxySettings
}

// Initialize initializes the resources proxy settings service needs.
// Call Close to close the resources and the proxy-settings page.
func (s *ProxySettingsService) Initialize(ctx context.Context, _ *emptypb.Empty) (_ *emptypb.Empty, retErr error) {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to get keyboard")
	}
	s.kb = kb

	return &emptypb.Empty{}, nil
}

// New starts up a new proxy setting service instance.
// Close must be called later to clean up the associated resources.
// Deprecated: use Initialize instead.
func (s *ProxySettingsService) New(ctx context.Context, _ *network.NewRequest) (_ *emptypb.Empty, retErr error) {
	return &emptypb.Empty{}, errors.New("rpc: tast.cros.network.ProxySettingService/New is deprecated, use tast.cros.network.ProxySettingService/Initialize instead")
}

// Close releases the resources obtained by New and closes the proxy-settings page.
func (s *ProxySettingsService) Close(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if s.proxySettings != nil {
		tconn, err := s.testAPIConn(ctx)
		if err != nil {
			return &emptypb.Empty{}, err
		}

		s.proxySettings.Close(ctx, tconn, s.kb)
		s.proxySettings = nil
	}

	if s.kb != nil {
		if err := s.kb.Close(ctx); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to close keyboard")
		}
		s.kb = nil
	}
	return &emptypb.Empty{}, nil
}

// ResetConnectionType resets the proxy settings by setting connection type to default value.
func (s *ProxySettingsService) ResetConnectionType(ctx context.Context, req *network.ResetConnectionTypeRequest) (*emptypb.Empty, error) {
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	if err := s.openProxySettingsPage(ctx, tconn, req.NetworkInfo); err != nil {
		return &emptypb.Empty{}, err
	}

	return &emptypb.Empty{}, s.proxySettings.SetDirectConnection(ctx, uiauto.New(tconn))
}

func (s *ProxySettingsService) testAPIConn(ctx context.Context) (*chrome.TestConn, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*chrome.TestConn, error) {
		return tconn, nil
	})
}

// dumpUITreeToFile dumps the UI tree to a specified file when there is an error.
func (s *ProxySettingsService) dumpUITreeToFile(ctx context.Context, hasError func() bool, nameSuffix string) {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.serviceState.Log("Failed to get output dir")
		return
	}
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		s.serviceState.Log("Failed to get Test API connection: ", err)
		return
	}
	faillog.DumpUITreeOnError(ctx, filepath.Join(outDir, "proxy_settings_service_"+nameSuffix), hasError, tconn)
}

// Setup sets up proxy values.
// Not specifying the SameHost/SamePort will ensure the toggle button "Use the same proxy for all protocols" being disabled.
// Note: It can not include other proxies when specifying the SameHost/SamePort.
func (s *ProxySettingsService) Setup(ctx context.Context, req *network.ProxyConfigs) (_ *emptypb.Empty, retErr error) {
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer s.dumpUITreeToFile(cleanupCtx, func() bool { return retErr != nil }, "ui_dump_setup")

	if err := s.openProxySettingsPage(ctx, tconn, req.NetworkInfo); err != nil {
		return &emptypb.Empty{}, err
	}
	switch req.ProxyConnectionType {
	case network.ProxyConnectionType_ManualProxyConfiguration:
		if err := s.proxySettings.SetManualConfig(ctx, tconn, s.kb, parseProxyConfigs(req)); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to setup the contents for proxy fields")
		}
	case network.ProxyConnectionType_DirectInternetConnection:
		if err := s.proxySettings.SetDirectConnection(ctx, uiauto.New(tconn)); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to set connection type")
		}
	default:
		return &emptypb.Empty{}, errors.Errorf("unexpected proxy connection type %v", req.ProxyConnectionType)
	}

	return &emptypb.Empty{}, nil
}

// FetchProxySettings returns proxy configurations.
func (s *ProxySettingsService) FetchProxySettings(ctx context.Context, req *network.FetchProxySettingsRequest) (_ *network.ProxyConfigs, retErr error) {
	result := &network.ProxyConfigs{NetworkInfo: &network.NetworkInfo{}}
	var networkName string
	switch val := req.NetworkInfo.Value.(type) {
	case *network.NetworkInfo_Ethernet:
		result.NetworkInfo.Value = &network.NetworkInfo_Ethernet{}
		networkName = "Ethernet" // The name of Ethernet on the cros_network_config page is different with shillconst.TypeEthernet.
	case *network.NetworkInfo_WifiSsid:
		result.NetworkInfo.Value = &network.NetworkInfo_WifiSsid{WifiSsid: val.WifiSsid}
		networkName = val.WifiSsid
	default:
		return nil, errors.Errorf("unknown network info: %+v", val)
	}

	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	if req.FetchSource == network.FetchProxySettingsRequest_CrosNetworkConfig {
		return s.fetchFromCrosNetworkConfig(ctx, tconn, result, networkName)
	}
	return s.fetchFromOSSettings(ctx, tconn, result, req.NetworkInfo)
}

func (s *ProxySettingsService) fetchFromOSSettings(ctx context.Context, tconn *chrome.TestConn, result *network.ProxyConfigs, networkInfo *network.NetworkInfo) (_ *network.ProxyConfigs, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer s.dumpUITreeToFile(cleanupCtx, func() bool { return retErr != nil }, "ui_dump_fetch_config")

	if err := s.openProxySettingsPage(ctx, tconn, networkInfo); err != nil {
		return nil, err
	}

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(ossettings.ProxyDropDownMenu)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait until drop down menu exists")
	}

	dropDownMenu, err := ui.Info(ctx, ossettings.ProxyDropDownMenu)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the value of proxy drop down menu")
	}

	// Proxy settings is available only when the connection type is 'Manual proxy configuration'.
	switch dropDownMenu.Value {
	case string(proxysettings.ManualProxyConfiguration):
		result.ProxyConnectionType = network.ProxyConnectionType_ManualProxyConfiguration
	case string(proxysettings.DirectInternetConnection):
		result.ProxyConnectionType = network.ProxyConnectionType_DirectInternetConnection
		return result, nil
	default:
		return nil, errors.Errorf("unknown or unsupported proxy connection type: %q", dropDownMenu.Value)
	}

	protocols := []proxysettings.Protocol{proxysettings.HTTP, proxysettings.HTTPS, proxysettings.Socks}
	if enable, err := s.proxySettings.IsUseSameProxyToggleOptionEnabled(ctx, tconn); err != nil {
		return nil, err
	} else if enable {
		protocols = []proxysettings.Protocol{proxysettings.SameProxy}
	}

	for _, protocol := range protocols {
		c, err := s.proxySettings.ManualConfigContent(ctx, tconn, protocol)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to acquire proxy configuration of %s", protocol.Name())
		}

		switch c.Protocol {
		case proxysettings.HTTP:
			result.HttpHost, result.HttpPort = c.Host, c.Port
		case proxysettings.HTTPS:
			result.HttpsHost, result.HttpsPort = c.Host, c.Port
		case proxysettings.Socks:
			result.SocksHost, result.SocksPort = c.Host, c.Port
		case proxysettings.SameProxy:
			result.SameHost, result.SamePort = c.Host, c.Port
		default:
			return nil, errors.Errorf("unknown protocol: %v", c.Protocol)
		}
	}

	return result, nil
}

// FetchConfigurations returns proxy hosts and ports.
// Deprecated: use FetchProxySettings instead.
func (s *ProxySettingsService) FetchConfigurations(ctx context.Context, _ *emptypb.Empty) (*network.ProxyConfigs, error) {
	return nil, errors.New("rpc: tast.cros.network.ProxySettingService/New is deprecated, use tast.cros.network.ProxySettingService/FetchProxySettings instead")
}

func (s *ProxySettingsService) fetchFromCrosNetworkConfig(ctx context.Context, tconn *chrome.TestConn, result *network.ProxyConfigs, networkName string) (_ *network.ProxyConfigs, retErr error) {
	s.sharedObject.ChromeMutex.Lock()
	defer s.sharedObject.ChromeMutex.Unlock()

	if s.sharedObject.Chrome == nil {
		return nil, errors.New("Chrome is not instantiated")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	createCrosNetconfig := netconfig.CreateLoggedInCrosNetworkConfig
	if s.sharedObject.Chrome.LoginMode() == "NoLogin" {
		createCrosNetconfig = netconfig.CreateOobeCrosNetworkConfig
	}

	crosNetworkConfig, err := createCrosNetconfig(ctx, s.sharedObject.Chrome)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cros network config")
	}
	defer crosNetworkConfig.Close(cleanupCtx)

	filter := netconfigtypes.NetworkFilter{Filter: netconfigtypes.ConfiguredFT}

	networkStateList, err := crosNetworkConfig.GetNetworkStateList(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network state list")
	}

	for _, networkProperties := range networkStateList {
		if networkProperties.Name != networkName {
			continue
		}

		managedProperties, err := crosNetworkConfig.GetManagedProperties(ctx, networkProperties.GUID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %q network managed properties", networkName)
		}

		if managedProperties.ProxySettings.Type.ActiveValue == "Direct" {
			result.ProxyConnectionType = network.ProxyConnectionType_DirectInternetConnection
			return result, nil
		}
		result.ProxyConnectionType = network.ProxyConnectionType_ManualProxyConfiguration

		result.HttpHost = managedProperties.ProxySettings.Manual.HTTPProxy.Host.ActiveValue
		result.HttpPort = strconv.Itoa(managedProperties.ProxySettings.Manual.HTTPProxy.Port.ActiveValue)

		result.HttpsHost = managedProperties.ProxySettings.Manual.SecureHTTPProxy.Host.ActiveValue
		result.HttpsPort = strconv.Itoa(managedProperties.ProxySettings.Manual.SecureHTTPProxy.Port.ActiveValue)

		result.SocksHost = managedProperties.ProxySettings.Manual.Socks.Host.ActiveValue
		result.SocksPort = strconv.Itoa(managedProperties.ProxySettings.Manual.Socks.Port.ActiveValue)

		return result, nil
	}
	return nil, errors.Errorf("failed to find %q network", networkName)
}

// SetException interacts/controls the exception domains in network detail page.
func (s *ProxySettingsService) SetException(ctx context.Context, req *network.SetExceptionRequest) (_ *empty.Empty, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer s.dumpUITreeToFile(cleanupCtx, func() bool { return retErr != nil }, "ui_dump_setup_exception")

	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		s.serviceState.Log("Failed to get Test API connection: ", err)
		return &emptypb.Empty{}, err
	}

	if err := s.openProxySettingsPage(ctx, tconn, req.NetworkInfo); err != nil {
		return nil, err
	}

	ui := uiauto.New(tconn)
	removeURLName := fmt.Sprintf("Remove exception for %s", req.Host)
	removeURL := nodewith.Role(role.Button).Name(removeURLName)
	if req.Action == network.SetExceptionRequest_REMOVE {
		return &empty.Empty{}, uiauto.Combine("remove exception domain",
			ui.WaitUntilExists(removeURL),
			ui.LeftClick(removeURL),
			ui.WaitUntilGone(nodewith.NameContaining(req.Host)),
			saveSettings(ui),
			ui.EnsureGoneFor(nodewith.Role(role.Button).NameStartingWith("Remove exception for "), 5*time.Second),
		)(ctx)
	}
	return &empty.Empty{}, uiauto.Combine("add an exception domain",
		ui.EnsureFocused(nodewith.Role(role.TextField).Name("Host or domain to exclude")),
		s.kb.TypeAction(req.Host),
		ui.LeftClick(nodewith.Role(role.Button).Name("Add exception")),
		ui.WaitUntilExists(removeURL),
		saveSettings(ui),
	)(ctx)
}

// FetchException returns exception settings.
func (s *ProxySettingsService) FetchException(ctx context.Context, req *network.FetchExceptionRequest) (*network.FetchExceptionResponse, error) {
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		s.serviceState.Log("Failed to get Test API connection: ", err)
		return nil, err
	}

	if err := s.openProxySettingsPage(ctx, tconn, req.NetworkInfo); err != nil {
		return nil, err
	}

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Role(role.TextField).Name("Host or domain to exclude"))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait exception field exists")
	}

	removeURLPrefix := "Remove exception for "
	removeURL := nodewith.Role(role.Button).NameStartingWith(removeURLPrefix)
	exceptionNodes, err := ui.NodesInfo(ctx, removeURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get nodes info")
	}

	var exceptions []string
	for _, exception := range exceptionNodes {
		exceptions = append(exceptions, strings.TrimLeft(exception.Name, removeURLPrefix))
	}
	return &network.FetchExceptionResponse{Exception: exceptions}, nil
}

// AllowProxiesForSharedNetwork allows or disallows proxies for shared networks by toggling the "Allow proxies for shared networks" toggle button.
func (s *ProxySettingsService) AllowProxiesForSharedNetwork(ctx context.Context, req *network.AllowProxiesForSharedNetworkRequest) (_ *empty.Empty, retErr error) {
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer s.dumpUITreeToFile(cleanupCtx, func() bool { return retErr != nil }, "allow_proxies_for_shared_network_ui_dump")

	if err := s.openProxySettingsPage(ctx, tconn, req.NetworkInfo); err != nil {
		return nil, err
	}
	defer s.proxySettings.Close(cleanupCtx, tconn, s.kb)

	if err := proxysettings.AllowProxiesForSharedNetwork(ctx, tconn, req.Allow); err != nil {
		return &emptypb.Empty{}, err
	}

	return &emptypb.Empty{}, nil
}

// IsProxySettingsRestricted checks whether or not the proxy settings section is restricted by
// checking the restriction state of a dropdown menu within the proxy settings section.
// This method should be called when DUT is Logged in.
func (s *ProxySettingsService) IsProxySettingsRestricted(ctx context.Context, req *network.IsProxySettingsRestrictedRequest) (_ *wrapperspb.BoolValue, retErr error) {
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		return &wrapperspb.BoolValue{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer s.dumpUITreeToFile(cleanupCtx, func() bool { return retErr != nil }, "is_proxy_restricted_ui_dump")

	s.sharedObject.ChromeMutex.Lock()
	defer s.sharedObject.ChromeMutex.Unlock()

	if s.sharedObject.Chrome == nil {
		return &wrapperspb.BoolValue{}, errors.New("Chrome in not initiated")
	}

	switch val := req.NetworkInfo.Value.(type) {
	case *network.NetworkInfo_Ethernet:
		if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
			return &wrapperspb.BoolValue{}, err
		}
		if err := quicksettings.OpenNetworkSettings(ctx, tconn); err != nil {
			return &wrapperspb.BoolValue{}, err
		}
	case *network.NetworkInfo_WifiSsid:
		if _, err := ossettings.OpenNetworkDetailPage(ctx, tconn, s.sharedObject.Chrome, val.WifiSsid, netconfigtypes.WiFi); err != nil {
			return &wrapperspb.BoolValue{}, err
		}
	}
	defer apps.Close(cleanupCtx, tconn, apps.Settings.ID)

	if err := proxysettings.ExpandProxySettingsSection(ctx, tconn); err != nil {
		return &wrapperspb.BoolValue{}, err
	}

	settings := ossettings.New(tconn)
	if err := settings.WaitUntilExists(ossettings.ProxyDropDownMenu)(ctx); err != nil {
		return &wrapperspb.BoolValue{}, errors.Wrap(err, "failed to find the proxy drop down menu")
	}

	info, err := settings.Info(ctx, ossettings.ProxyDropDownMenu)
	if err != nil {
		return &wrapperspb.BoolValue{}, errors.Wrap(err, "failed to get the information about proxy drop down menu")
	}

	return &wrapperspb.BoolValue{Value: info.Restriction != restriction.Disabled}, nil
}

// openProxySettingsPage opens proxy settings page of the specified network.
func (s *ProxySettingsService) openProxySettingsPage(ctx context.Context, tconn *chrome.TestConn, networkInfo *network.NetworkInfo) error {
	s.sharedObject.ChromeMutex.Lock()
	defer s.sharedObject.ChromeMutex.Unlock()

	cr := s.sharedObject.Chrome
	if cr == nil {
		return errors.New("Chrome has not been started")
	}

	if s.proxySettings != nil {
		return nil
	}

	if networkInfo == nil {
		return errors.New("missing 'NetworkInfo' field")
	}

	isLoggedin := s.sharedObject.Chrome.LoginMode() != "NoLogin"

	var err error
	switch val := networkInfo.Value.(type) {
	case *network.NetworkInfo_Ethernet:
		s.proxySettings, err = proxysettings.CollectEthernet(ctx, tconn, isLoggedin)
	case *network.NetworkInfo_WifiSsid:
		s.proxySettings, err = proxysettings.CollectWifi(ctx, cr, tconn, val.WifiSsid, isLoggedin)
	default:
		err = errors.Errorf("unknown network info: %+v", val)
	}
	if err != nil {
		s.proxySettings = nil
		return err
	}

	return nil
}

// parseProxyConfigs parses ProxyConfig from network grpc service into local proxysettings config.
func parseProxyConfigs(req *network.ProxyConfigs) []*proxysettings.Config {
	if req.SameHost != "" {
		return []*proxysettings.Config{
			{Protocol: proxysettings.SameProxy, Host: req.SameHost, Port: req.SamePort},
		}
	}
	return []*proxysettings.Config{
		{Protocol: proxysettings.HTTP, Host: req.HttpHost, Port: req.HttpPort},
		{Protocol: proxysettings.HTTPS, Host: req.HttpsHost, Port: req.HttpsPort},
		{Protocol: proxysettings.Socks, Host: req.SocksHost, Port: req.SocksPort},
	}
}

func saveSettings(ui *uiauto.Context) uiauto.Action {
	saveButton := ossettings.WindowFinder.HasClass("action-button").Name("Save").Role(role.Button)
	return func(ctx context.Context) error {
		return uiauto.Combine("save settings",
			// Verify the save button gets enabled.
			ui.CheckRestriction(ossettings.WindowFinder.HasClass("action-button").Name("Save").Role(role.Button), restriction.None),
			ui.MakeVisible(saveButton),
			ui.WaitForLocation(saveButton),
			ui.WithInterval(time.Second).LeftClickUntil(saveButton, ui.CheckRestriction(saveButton, restriction.Disabled)),
		)(ctx)
	}
}
