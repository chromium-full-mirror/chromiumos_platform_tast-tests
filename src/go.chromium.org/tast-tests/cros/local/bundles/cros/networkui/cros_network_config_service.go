// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/networkui/netconfig"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			networkui.RegisterCrosNetworkConfigServiceServer(srv, &CrosNetworkConfigService{
				serviceState: s,
				sharedObject: common.SharedObjectsForServiceSingleton,
			})
		},
	})
}

// CrosNetworkConfigService implements tast.cros.networkui.CrosNetworkConfigService.
// The mojo connection of the cros_network_config will be handled internally by each method,
// caller will not be responsible for managing it.
type CrosNetworkConfigService struct {
	sharedObject *common.SharedObjectsForService
	serviceState *testing.ServiceState
}

// ConfigureNetwork configures the network by CrosNetworkConfig mojo API.
func (s *CrosNetworkConfigService) ConfigureNetwork(ctx context.Context, req *networkui.ConfigureNetworkRequest) (*networkui.ConfigureNetworkResponse, error) {
	s.sharedObject.ChromeMutex.Lock()
	defer s.sharedObject.ChromeMutex.Unlock()

	if s.sharedObject.Chrome == nil {
		return nil, errors.New("Chrome is not instantiated")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	createCrosNetConfig := netconfig.CreateLoggedInCrosNetworkConfig
	if s.sharedObject.Chrome.LoginMode() == "NoLogin" {
		createCrosNetConfig = netconfig.CreateOobeCrosNetworkConfig
	}

	crosNetConfig, err := createCrosNetConfig(ctx, s.sharedObject.Chrome)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cros network config")
	}
	defer crosNetConfig.Close(cleanupCtx)

	configProperties, err := parseNetConfigProperties(req.ConfigProperties)
	if err != nil {
		return nil, err
	}

	guid, err := crosNetConfig.ConfigureNetwork(ctx, *configProperties, req.Shared)
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure the network")
	}

	return &networkui.ConfigureNetworkResponse{Guid: guid}, nil
}

// GetNetworkStateList returns an array of states of networks based on the filter.
func (s *CrosNetworkConfigService) GetNetworkStateList(ctx context.Context, req *networkui.NetworkFilter) (*networkui.GetNetworkStateListResponse, error) {
	s.sharedObject.ChromeMutex.Lock()
	defer s.sharedObject.ChromeMutex.Unlock()

	if s.sharedObject.Chrome == nil {
		return nil, errors.New("Chrome is not instantiated")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	createCrosNetConfig := netconfig.CreateLoggedInCrosNetworkConfig
	if s.sharedObject.Chrome.LoginMode() == "NoLogin" {
		createCrosNetConfig = netconfig.CreateOobeCrosNetworkConfig
	}

	crosNetConfig, err := createCrosNetConfig(ctx, s.sharedObject.Chrome)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cros network config")
	}
	defer crosNetConfig.Close(cleanupCtx)

	properties, err := crosNetConfig.GetNetworkStateList(ctx, parseNetworkFilter(req))
	if err != nil {
		return nil, errors.Wrap(err, "failed get network state list")
	}

	results, err := parseNetworkStateProperties(properties)
	if err != nil {
		return nil, err
	}

	return &networkui.GetNetworkStateListResponse{Properties: results}, nil
}

// parseNetworkFilter parses the NetworkFilter in network grpc service to the local netconfig data struct.
func parseNetworkFilter(f *networkui.NetworkFilter) netconfigtypes.NetworkFilter {
	var networkFilter netconfigtypes.NetworkFilter
	switch f.GetFilter() {
	case networkui.FilterType_ALL_FILTER_TYPE:
		networkFilter.Filter = netconfigtypes.AllFT
	case networkui.FilterType_ACTIVE:
		networkFilter.Filter = netconfigtypes.ActiveFT
	case networkui.FilterType_VISIBLE:
		networkFilter.Filter = netconfigtypes.VisibleFT
	case networkui.FilterType_CONFIGURED:
		networkFilter.Filter = netconfigtypes.ConfiguredFT
	default:
		networkFilter.Filter = netconfigtypes.AllFT
	}

	switch f.GetNetwork() {
	case networkui.NetworkType_ALL_NETWORK_TYPE:
		networkFilter.NetworkType = netconfigtypes.All
	case networkui.NetworkType_CELLULAR:
		networkFilter.NetworkType = netconfigtypes.Cellular
	case networkui.NetworkType_ETHERNET:
		networkFilter.NetworkType = netconfigtypes.Ethernet
	case networkui.NetworkType_MOBILE:
		networkFilter.NetworkType = netconfigtypes.Mobile
	case networkui.NetworkType_TETHER:
		networkFilter.NetworkType = netconfigtypes.Tether
	case networkui.NetworkType_VPN:
		networkFilter.NetworkType = netconfigtypes.VPN
	case networkui.NetworkType_WIRELESS:
		networkFilter.NetworkType = netconfigtypes.Wireless
	case networkui.NetworkType_WIFI:
		networkFilter.NetworkType = netconfigtypes.WiFi
	default:
		networkFilter.NetworkType = netconfigtypes.All
	}

	networkFilter.Limit = f.GetLimit()
	return networkFilter
}

// parseNetworkStateProperties parses a slice of NetworkStateProperties to the grpc service data struct.
func parseNetworkStateProperties(properties []netconfigtypes.NetworkStateProperties) ([]*networkui.NetworkStateProperties, error) {
	results := make([]*networkui.NetworkStateProperties, len(properties))
	for i, property := range properties {
		results[i] = &networkui.NetworkStateProperties{
			Name: property.Name,
		}
		switch property.ConnectionState {
		case netconfigtypes.OnlineCST:
			results[i].ConnectionState = networkui.ConnectionState_ONLINE
		case netconfigtypes.ConnectedCST:
			results[i].ConnectionState = networkui.ConnectionState_CONNECTED
		case netconfigtypes.PortalCST:
			results[i].ConnectionState = networkui.ConnectionState_PORTAL
		case netconfigtypes.ConnectingCST:
			results[i].ConnectionState = networkui.ConnectionState_CONNECTING
		case netconfigtypes.NotConnectedCST:
			results[i].ConnectionState = networkui.ConnectionState_NOT_CONNECTED
		default:
			return nil, errors.Errorf("Unrecognized connection state: %v", property.ConnectionState)
		}
	}
	return results, nil
}

// parseNetConfigProperties parses the NetworkTypeConfigProperties in network grpc service to the local netconfig data struct.
func parseNetConfigProperties(configProperties *networkui.NetworkTypeConfigProperties) (*netconfigtypes.ConfigProperties, error) {
	result := &netconfigtypes.ConfigProperties{}

	if configProperties.AutoConnect != nil {
		result.AutoConnect = netconfigtypes.AutoConnectConfigProperties{
			Value: configProperties.AutoConnect.Value,
		}
	}

	// Currently only the WiFi network is supported.
	// It can be further extended by adding other types of network: VPN/Ethernet, see: netconfig.NetworkTypeConfigProperties.
	switch properties := configProperties.ConfigProperties.(type) {
	case *networkui.NetworkTypeConfigProperties_WifiConfigProperties:
		result.TypeConfig.Wifi = parseWifiSecurity(properties.WifiConfigProperties)
		result.TypeConfig.Wifi.HiddenSsid = parseWifiHiddenSsidMode(properties.WifiConfigProperties.HiddenSsidMode)
		return result, nil
	default:
		return nil, errors.New("failed to parse the network type config properties")
	}
}

// parseWifiSecurity parses the security type of the WiFi config in network grpc service to the local netconfig data struct.
func parseWifiSecurity(wifi *networkui.WiFiConfigProperties) *netconfigtypes.WiFiConfigProperties {
	properties := &netconfigtypes.WiFiConfigProperties{Ssid: wifi.Ssid}

	switch wifi.Security.(type) {
	case *networkui.WiFiConfigProperties_None:
		properties.Security = netconfigtypes.None
	case *networkui.WiFiConfigProperties_Psk:
		properties.Security = netconfigtypes.WpaPsk
		properties.Passphrase = wifi.GetPsk()
	}
	return properties
}

// parseWifiHiddenSsidMode parses the HiddenSSIDMode in network grpc service to the local netconfig data struct.
func parseWifiHiddenSsidMode(hiddenMode networkui.HiddenSSIDMode) (mode netconfigtypes.HiddenSsidMode) {
	switch hiddenMode {
	case networkui.HiddenSSIDMode_Automatic:
		mode = netconfigtypes.Automatic
	case networkui.HiddenSSIDMode_Disabled:
		mode = netconfigtypes.Disabled
	case networkui.HiddenSSIDMode_Enabled:
		mode = netconfigtypes.Enabled
	}
	return
}
