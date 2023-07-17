// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast-tests/cros/services/cros/network"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			network.RegisterCrosNetworkConfigServiceServer(srv, &CrosNetworkConfigService{
				serviceState: s,
				sharedObject: common.SharedObjectsForServiceSingleton,
			})
		},
	})
}

// CrosNetworkConfigService implements tast.cros.network.CrosNetworkConfigService.
// The mojo connection of the cros_network_config will be handled internally by each method,
// caller will not be responsible for managing it.
type CrosNetworkConfigService struct {
	sharedObject *common.SharedObjectsForService
	serviceState *testing.ServiceState
}

// ConfigureNetwork configures the network by CrosNetworkConfig mojo API.
func (s *CrosNetworkConfigService) ConfigureNetwork(ctx context.Context, req *network.ConfigureNetworkRequest) (*network.ConfigureNetworkResponse, error) {
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

	return &network.ConfigureNetworkResponse{Guid: guid}, nil
}

// parseNetConfigProperties parses the NetworkTypeConfigProperties in network grpc service to the local netconfig data struct.
func parseNetConfigProperties(configProperties *network.NetworkTypeConfigProperties) (*netconfigtypes.ConfigProperties, error) {
	result := &netconfigtypes.ConfigProperties{}

	if configProperties.AutoConnect != nil {
		result.AutoConnect = netconfigtypes.AutoConnectConfigProperties{
			Value: configProperties.AutoConnect.Value,
		}
	}

	// Currently only the WiFi network is supported.
	// It can be further extended by adding other types of network: VPN/Ethernet, see: netconfig.NetworkTypeConfigProperties.
	switch properties := configProperties.ConfigProperties.(type) {
	case *network.NetworkTypeConfigProperties_WifiConfigProperties:
		result.TypeConfig.Wifi = parseWifiSecurity(properties.WifiConfigProperties)
		result.TypeConfig.Wifi.HiddenSsid = parseWifiHiddenSsidMode(properties.WifiConfigProperties.HiddenSsidMode)
		return result, nil
	default:
		return nil, errors.New("failed to parse the network type config properties")
	}
}

// parseWifiSecurity parses the security type of the WiFi config in network grpc service to the local netconfig data struct.
func parseWifiSecurity(wifi *network.WiFiConfigProperties) *netconfigtypes.WiFiConfigProperties {
	properties := &netconfigtypes.WiFiConfigProperties{Ssid: wifi.Ssid}

	switch wifi.Security.(type) {
	case *network.WiFiConfigProperties_None:
		properties.Security = netconfigtypes.None
	case *network.WiFiConfigProperties_Psk:
		properties.Security = netconfigtypes.WpaPsk
		properties.Passphrase = wifi.GetPsk()
	}
	return properties
}

// parseWifiHiddenSsidMode parses the HiddenSSIDMode in network grpc service to the local netconfig data struct.
func parseWifiHiddenSsidMode(hiddenMode network.HiddenSSIDMode) (mode netconfigtypes.HiddenSsidMode) {
	switch hiddenMode {
	case network.HiddenSSIDMode_Automatic:
		mode = netconfigtypes.Automatic
	case network.HiddenSSIDMode_Disabled:
		mode = netconfigtypes.Disabled
	case network.HiddenSSIDMode_Enabled:
		mode = netconfigtypes.Enabled
	}
	return
}
