// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/network/proxysettings"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/common"
	"chromiumos/tast/local/input"
	"chromiumos/tast/services/cros/network"
	"chromiumos/tast/testing"
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
		if err := s.kb.Close(); err != nil {
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

	if err := s.prepareProxySettingsPage(ctx, tconn, req.NetworkInfo); err != nil {
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
		testing.ContextLog(ctx, "Failed to get output dir")
		return
	}
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		s.serviceState.Log(ctx, "Failed to get Test API connection: ", err)
		return
	}
	faillog.DumpUITreeOnError(ctx, filepath.Join(outDir, "proxy_settings_service_"+nameSuffix), hasError, tconn)
}

// Setup sets up proxy values.
func (s *ProxySettingsService) Setup(ctx context.Context, req *network.ProxyConfigs) (_ *emptypb.Empty, retErr error) {
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer s.dumpUITreeToFile(cleanupCtx, func() bool { return retErr != nil }, "ui_dump_setup")

	if err := s.prepareProxySettingsPage(ctx, tconn, req.NetworkInfo); err != nil {
		return &emptypb.Empty{}, err
	}
	switch req.ProxyConnectionType {
	case network.ProxyConnectionType_ManualProxyConfiguration:
		if err := s.proxySettings.SetManualConfig(ctx, tconn, s.kb, s.parseProxyConfigs(req)); err != nil {
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
	tconn, err := s.testAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer s.dumpUITreeToFile(cleanupCtx, func() bool { return retErr != nil }, "ui_dump_fetch_config")

	if err := s.prepareProxySettingsPage(ctx, tconn, req.NetworkInfo); err != nil {
		return nil, err
	}

	result := &network.ProxyConfigs{NetworkInfo: &network.NetworkInfo{}}
	switch val := req.NetworkInfo.Value.(type) {
	case *network.NetworkInfo_Ethernet:
		result.NetworkInfo.Value = &network.NetworkInfo_Ethernet{}
	case *network.NetworkInfo_WifiSsid:
		result.NetworkInfo.Value = &network.NetworkInfo_WifiSsid{WifiSsid: val.WifiSsid}
	default:
		return nil, errors.Errorf("unknown network info: %+v", val)
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

	for _, protocol := range []proxysettings.Protocol{
		proxysettings.HTTP, proxysettings.HTTPS, proxysettings.Socks,
	} {
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

// parseProxyConfigs parses ProxyConfig from network grpc service into local proxysettings config.
func (s *ProxySettingsService) parseProxyConfigs(req *network.ProxyConfigs) []*proxysettings.Config {
	return []*proxysettings.Config{
		{Protocol: proxysettings.HTTP, Host: req.HttpHost, Port: req.HttpPort},
		{Protocol: proxysettings.HTTPS, Host: req.HttpsHost, Port: req.HttpsPort},
		{Protocol: proxysettings.Socks, Host: req.SocksHost, Port: req.SocksPort},
	}
}

// prepareProxySettingsPage opens proxy settings page of the specified network.
func (s *ProxySettingsService) prepareProxySettingsPage(ctx context.Context, tconn *chrome.TestConn, networkInfo *network.NetworkInfo) error {
	if s.proxySettings != nil {
		return nil
	}

	if networkInfo == nil {
		return errors.New("missing 'NetworkInfo' field")
	}

	var err error
	switch val := networkInfo.Value.(type) {
	case *network.NetworkInfo_Ethernet:
		s.proxySettings, err = proxysettings.CollectEthernet(ctx, tconn, false /*isLoggedIn*/)
	case *network.NetworkInfo_WifiSsid:
		s.sharedObject.ChromeMutex.Lock()
		defer s.sharedObject.ChromeMutex.Unlock()

		cr := s.sharedObject.Chrome
		if cr == nil {
			return errors.New("Chrome has not been started")
		}
		s.proxySettings, err = proxysettings.CollectWifi(ctx, cr, tconn, val.WifiSsid, false /*isLoggedIn*/)
	default:
		err = errors.Errorf("unknown network info: %+v", val)
	}
	if err != nil {
		s.proxySettings = nil
		return err
	}

	return nil
}
