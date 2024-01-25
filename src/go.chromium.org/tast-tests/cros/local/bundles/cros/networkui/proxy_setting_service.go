// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/networkui/proxysettings"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/networkui/netconfig"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			networkui.RegisterProxySettingServiceServer(srv, &ProxySettingsService{
				serviceState: s,
				sharedObject: common.SharedObjectsForServiceSingleton,
			})
		},
	})
}

// ProxySettingsService implements tast.cros.networkui.ProxySettingsService.
type ProxySettingsService struct {
	sharedObject *common.SharedObjectsForService
	serviceState *testing.ServiceState
}

// ResetConnectionType resets the proxy setting connection type, can be used to cleanup the proxy settings.
func (s *ProxySettingsService) ResetConnectionType(ctx context.Context, req *networkui.ResetConnectionTypeRequest) (_ *emptypb.Empty, retErr error) {
	cr, tconn, manager, target, err := s.initRuntimeResources(ctx, req.GetLoginMode(), req.GetNetworkInfo())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := manager.LaunchAndPrepare(ctx, cr, tconn, target); err != nil {
		return &emptypb.Empty{}, err
	}
	defer manager.Close(cleanupCtx, cr, tconn)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "ui_dump_reset_connection_type")

	return &emptypb.Empty{}, manager.SetDirectConnection(ctx, uiauto.New(tconn))
}

// SetProxySettings sets up proxy values.
// Not specifying the SameHost/SamePort will ensure the toggle button "Use the same proxy for all protocols" being disabled.
// Note: It can not include other proxies when specifying the SameHost/SamePort.
func (s *ProxySettingsService) SetProxySettings(ctx context.Context, req *networkui.SetProxySettingsRequest) (_ *emptypb.Empty, retErr error) {
	configs := req.GetConfigs()
	cr, tconn, manager, target, err := s.initRuntimeResources(ctx, req.GetLoginMode(), configs.GetNetworkInfo())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}
	defer kb.Close(cleanupCtx)

	if err := manager.LaunchAndPrepare(ctx, cr, tconn, target); err != nil {
		return &emptypb.Empty{}, err
	}
	defer manager.Close(cleanupCtx, cr, tconn)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "ui_dump_setup_proxy")

	switch configs.ProxyConnectionType {
	case networkui.ProxyConnectionType_ManualProxyConfiguration:
		if err := manager.SetManualConfig(ctx, tconn, kb, parseProxyConfigs(configs)); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to setup the contents for proxy fields")
		}
	case networkui.ProxyConnectionType_DirectInternetConnection:
		if err := manager.SetDirectConnection(ctx, uiauto.New(tconn)); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to set connection type")
		}
	default:
		return &emptypb.Empty{}, errors.Errorf("unexpected proxy connection type %v", configs.ProxyConnectionType)
	}

	return &emptypb.Empty{}, nil
}

// FetchProxySettings returns proxy configurations.
func (s *ProxySettingsService) FetchProxySettings(ctx context.Context, req *networkui.FetchProxySettingsRequest) (_ *networkui.ProxyConfigs, retErr error) {
	cr, tconn, manager, target, err := s.initRuntimeResources(ctx, req.GetLoginMode(), req.GetNetworkInfo())
	if err != nil {
		return nil, err
	}

	var fetchFunc func(context.Context, *chrome.Chrome, *chrome.TestConn, *proxysettings.Manager, proxysettings.TargetNetwork) (*networkui.ProxyConfigs, error)
	switch fetchSource := req.GetFetchSource(); fetchSource {
	case networkui.FetchProxySettingsRequest_CrosNetworkConfig:
		fetchFunc = s.fetchFromCrosNetworkConfig
	case networkui.FetchProxySettingsRequest_OSSettings:
		fetchFunc = s.fetchFromOSSettings
	default:
		return nil, errors.Errorf("unrecognized fetch source %v", fetchSource)
	}

	configs, err := fetchFunc(ctx, cr, tconn, manager, target)
	if err != nil {
		return nil, err
	}
	configs.NetworkInfo = req.GetNetworkInfo()

	return configs, nil
}

func (s *ProxySettingsService) fetchFromOSSettings(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, manager *proxysettings.Manager, target proxysettings.TargetNetwork) (_ *networkui.ProxyConfigs, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := manager.Launch(ctx, cr, tconn, target); err != nil {
		return nil, err
	}
	defer manager.Close(cleanupCtx, cr, tconn)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "ui_dump_fetch")

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(ossettings.ProxyDropDownMenu)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait until drop down menu exists")
	}

	dropDownMenu, err := ui.Info(ctx, ossettings.ProxyDropDownMenu)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the value of proxy drop down menu")
	}

	result := &networkui.ProxyConfigs{}

	// Proxy settings is available only when the connection type is 'Manual proxy configuration'.
	switch dropDownMenu.Value {
	case string(proxysettings.ManualProxyConfiguration):
		result.ProxyConnectionType = networkui.ProxyConnectionType_ManualProxyConfiguration
	case string(proxysettings.DirectInternetConnection):
		result.ProxyConnectionType = networkui.ProxyConnectionType_DirectInternetConnection
		return result, nil
	default:
		return nil, errors.Errorf("unknown or unsupported proxy connection type: %q", dropDownMenu.Value)
	}

	protocols := []proxysettings.Protocol{proxysettings.HTTP, proxysettings.HTTPS, proxysettings.Socks}
	if enable, err := manager.IsUseSameProxyToggleOptionEnabled(ctx, tconn); err != nil {
		return nil, err
	} else if enable {
		protocols = []proxysettings.Protocol{proxysettings.SameProxy}
	}

	for _, protocol := range protocols {
		c, err := manager.ManualConfigContent(ctx, tconn, protocol)
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

func (s *ProxySettingsService) fetchFromCrosNetworkConfig(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, manager *proxysettings.Manager, target proxysettings.TargetNetwork) (_ *networkui.ProxyConfigs, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	createCrosNetconfig := netconfig.CreateLoggedInCrosNetworkConfig
	if cr.LoginMode() == "NoLogin" {
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

	result := &networkui.ProxyConfigs{}

	for _, networkProperties := range networkStateList {
		if networkProperties.Name != target.Name() {
			continue
		}

		managedProperties, err := crosNetworkConfig.GetManagedProperties(ctx, networkProperties.GUID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %q network managed properties", target.Name())
		}

		switch proxyConnectType := managedProperties.ProxySettings.Type.ActiveValue; proxyConnectType {
		// The default of proxy settings is "Direct Internet Connection",
		// it won't display any settings if the settings hasn't been changed,
		// however, it will display as "Direct" if switched from another settings.
		case "", "Direct":
			result.ProxyConnectionType = networkui.ProxyConnectionType_DirectInternetConnection
		case "Manual":
			result.ProxyConnectionType = networkui.ProxyConnectionType_ManualProxyConfiguration

			result.HttpHost = managedProperties.ProxySettings.Manual.HTTPProxy.Host.ActiveValue
			result.HttpPort = strconv.Itoa(managedProperties.ProxySettings.Manual.HTTPProxy.Port.ActiveValue)

			result.HttpsHost = managedProperties.ProxySettings.Manual.SecureHTTPProxy.Host.ActiveValue
			result.HttpsPort = strconv.Itoa(managedProperties.ProxySettings.Manual.SecureHTTPProxy.Port.ActiveValue)

			result.SocksHost = managedProperties.ProxySettings.Manual.Socks.Host.ActiveValue
			result.SocksPort = strconv.Itoa(managedProperties.ProxySettings.Manual.Socks.Port.ActiveValue)
		default:
			return nil, errors.Errorf("unsupported proxy type %v", proxyConnectType)
		}

		return result, nil
	}
	return nil, errors.Errorf("failed to find %q network", target.Name())
}

// SetException interacts/controls the exception domains in network detail page.
func (s *ProxySettingsService) SetException(ctx context.Context, req *networkui.SetExceptionRequest) (_ *empty.Empty, retErr error) {
	cr, tconn, manager, target, err := s.initRuntimeResources(ctx, req.GetLoginMode(), req.GetNetworkInfo())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}
	defer kb.Close(cleanupCtx)

	if err := manager.LaunchAndPrepare(ctx, cr, tconn, target); err != nil {
		return &emptypb.Empty{}, err
	}
	defer manager.Close(cleanupCtx, cr, tconn)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "ui_dump_set_exception")

	ui := uiauto.New(tconn)
	removeURLName := fmt.Sprintf("Remove exception for %s", req.Host)
	removeURL := nodewith.Role(role.Button).Name(removeURLName)
	if req.Action == networkui.SetExceptionRequest_REMOVE {
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
		kb.TypeAction(req.Host),
		ui.LeftClick(nodewith.Role(role.Button).Name("Add exception")),
		ui.WaitUntilExists(removeURL),
		saveSettings(ui),
	)(ctx)
}

// FetchException returns exception settings.
func (s *ProxySettingsService) FetchException(ctx context.Context, req *networkui.FetchExceptionRequest) (_ *networkui.FetchExceptionResponse, retErr error) {
	cr, tconn, manager, target, err := s.initRuntimeResources(ctx, req.GetLoginMode(), req.GetNetworkInfo())
	if err != nil {
		return nil, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := manager.Launch(ctx, cr, tconn, target); err != nil {
		return nil, err
	}
	defer manager.Close(cleanupCtx, cr, tconn)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "ui_dump_fetch_exception")

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Role(role.TextField).Name("Host or domain to exclude"))(ctx); err != nil {
		if strings.Contains(err.Error(), nodewith.ErrNotFound) {
			return &networkui.FetchExceptionResponse{Exception: []string{}}, nil
		}
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
		exceptions = append(exceptions, strings.TrimPrefix(exception.Name, removeURLPrefix))
	}
	return &networkui.FetchExceptionResponse{Exception: exceptions}, nil
}

// AllowProxiesForSharedNetwork allows or disallows proxies for shared networks by toggling the "Allow proxies for shared networks" toggle button.
func (s *ProxySettingsService) AllowProxiesForSharedNetwork(ctx context.Context, req *networkui.AllowProxiesForSharedNetworkRequest) (_ *empty.Empty, retErr error) {
	cr, tconn, manager, target, err := s.initRuntimeResources(ctx, req.GetLoginMode(), req.GetNetworkInfo())
	if err != nil {
		return nil, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := manager.Launch(ctx, cr, tconn, target); err != nil {
		return nil, err
	}
	defer manager.Close(cleanupCtx, cr, tconn)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "ui_dump_allow_proxies_for_shared_network")

	if err := proxysettings.AllowProxiesForSharedNetwork(ctx, tconn, req.Allow); err != nil {
		return &emptypb.Empty{}, err
	}

	return &emptypb.Empty{}, nil
}

// IsProxySettingsRestricted checks whether or not the proxy settings section is restricted by
// checking the restriction state of a dropdown menu within the proxy settings section.
// This method should be called when DUT is Logged in.
func (s *ProxySettingsService) IsProxySettingsRestricted(ctx context.Context, req *networkui.IsProxySettingsRestrictedRequest) (_ *wrapperspb.BoolValue, retErr error) {
	cr, tconn, manager, target, err := s.initRuntimeResources(ctx, req.GetLoginMode(), req.GetNetworkInfo())
	if err != nil {
		return nil, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := manager.Launch(ctx, cr, tconn, target); err != nil {
		return nil, err
	}
	defer manager.Close(cleanupCtx, cr, tconn)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "ui_dump_is_proxy_restricted")

	settings := ossettings.New(tconn)
	if err := settings.WaitUntilExists(ossettings.ProxyDropDownMenu)(ctx); err != nil {
		return &wrapperspb.BoolValue{}, errors.Wrap(err, "failed to find the proxy drop down menu")
	}

	info, err := settings.Info(ctx, ossettings.ProxyDropDownMenu)
	if err != nil {
		return &wrapperspb.BoolValue{}, errors.Wrap(err, "failed to get the information about proxy drop down menu")
	}

	return &wrapperspb.BoolValue{Value: info.Restriction == restriction.Disabled}, nil
}

func (s *ProxySettingsService) initRuntimeResources(ctx context.Context, loginMode networkui.LoginMode, networkInfo *networkui.NetworkInfo) (*chrome.Chrome, *chrome.TestConn, *proxysettings.Manager, proxysettings.TargetNetwork, error) {
	if s.sharedObject.Chrome == nil {
		return nil, nil, nil, nil, errors.New("Chrome is not instantiated")
	}

	tconn, err := common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*chrome.TestConn, error) {
		return tconn, nil
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}

	manager, err := s.initProxyManager(loginMode)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	target, err := s.parseTargetNetwork(networkInfo)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return s.sharedObject.Chrome, tconn, manager, target, nil
}

func (s *ProxySettingsService) initProxyManager(loginMode networkui.LoginMode) (*proxysettings.Manager, error) {
	switch loginMode {
	case networkui.LoginMode_LoggedIn:
		return proxysettings.NewProxySettingsManager(proxysettings.LoggedIn), nil
	case networkui.LoginMode_OOBE:
		return proxysettings.NewProxySettingsManager(proxysettings.OOBE), nil
	case networkui.LoginMode_SignInScreen:
		return proxysettings.NewProxySettingsManager(proxysettings.SignInScreen), nil
	default:
		return nil, errors.Errorf("unrecognized login mode %v", loginMode)
	}
}

func (s *ProxySettingsService) parseTargetNetwork(networkInfo *networkui.NetworkInfo) (proxysettings.TargetNetwork, error) {
	switch val := networkInfo.Value.(type) {
	case *networkui.NetworkInfo_Ethernet:
		return proxysettings.Ethernet(), nil
	case *networkui.NetworkInfo_WifiSsid:
		return proxysettings.Wifi(val.WifiSsid), nil
	default:
		return nil, errors.Errorf("unrecognized target network %v", val)
	}
}

// parseProxyConfigs parses ProxyConfig from network grpc service into local proxysettings config.
func parseProxyConfigs(req *networkui.ProxyConfigs) []*proxysettings.Config {
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

// Initialize initializes the resources proxy settings service needs.
// Call Close to close the resources and the proxy-settings page.
//
// Deprecated: this RPC is deprecated.
func (s *ProxySettingsService) Initialize(ctx context.Context, _ *emptypb.Empty) (_ *emptypb.Empty, retErr error) {
	return nil, errors.New("rpc: tast.cros.networkui.ProxySettingService/Initialize is deprecated")
}

// New starts up a new proxy setting service instance.
// Close must be called later to clean up the associated resources.
//
// Deprecated: this RPC is deprecated.
func (s *ProxySettingsService) New(ctx context.Context, _ *networkui.NewRequest) (_ *emptypb.Empty, retErr error) {
	return nil, errors.New("rpc: tast.cros.networkui.ProxySettingService/New is deprecated")
}

// Close releases the resources obtained by New and closes the proxy-settings page.
//
// Deprecated: this RPC is deprecated.
func (s *ProxySettingsService) Close(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return nil, errors.New("rpc: tast.cros.networkui.ProxySettingService/Close is deprecated")
}

// Setup sets up proxy values.
// Not specifying the SameHost/SamePort will ensure the toggle button "Use the same proxy for all protocols" being disabled.
// Note: It can not include other proxies when specifying the SameHost/SamePort.
//
// Deprecated: use SetProxySettings instead.
func (s *ProxySettingsService) Setup(ctx context.Context, req *networkui.ProxyConfigs) (_ *emptypb.Empty, retErr error) {
	return nil, errors.New("rpc: tast.cros.networkui.ProxySettingService/Setup is deprecated, use tast.cros.networkui.ProxySettingService/SetProxySettings instead")
}

// FetchConfigurations returns proxy hosts and ports.
//
// Deprecated: use FetchProxySettings instead.
func (s *ProxySettingsService) FetchConfigurations(ctx context.Context, _ *emptypb.Empty) (*networkui.ProxyConfigs, error) {
	return nil, errors.New("rpc: tast.cros.networkui.ProxySettingService/New is deprecated, use tast.cros.networkui.ProxySettingService/FetchProxySettings instead")
}
