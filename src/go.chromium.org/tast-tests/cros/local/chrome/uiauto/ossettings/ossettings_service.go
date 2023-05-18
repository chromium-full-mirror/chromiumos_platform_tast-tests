// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var osSettingsService Service
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			osSettingsService = Service{sharedObject: common.SharedObjectsForServiceSingleton, serviceState: s}
			pb.RegisterOsSettingsServiceServer(srv, &osSettingsService)
		},
	})
}

// Service implements tast.cros.chrome.uiauto.ossettings.OsSettingsService
type Service struct {
	serviceState *testing.ServiceState
	sharedObject *common.SharedObjectsForService
}

func computeNetworkConfigNetworkType(networkType pb.OpenNetworkDetailPageRequest_NetworkType) (netconfigtypes.NetworkType, error) {
	if networkType == pb.OpenNetworkDetailPageRequest_CELLULAR {
		return netconfigtypes.Cellular, nil
	}
	if networkType == pb.OpenNetworkDetailPageRequest_WIFI {
		return netconfigtypes.WiFi, nil
	}
	return 0, errors.New("Network type must be Cellular or WiFi")
}

// LaunchAtNetwork will launch the OS Settings application at Network page.
func (s *Service) LaunchAtNetwork(ctx context.Context, e *emptypb.Empty) (*emptypb.Empty, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		_, err := LaunchAtPage(ctx, tconn, Network)
		return &emptypb.Empty{}, err
	})
}

// LaunchAtWifiPage will launch the OS Settings application at Wi-Fi page.
func (s *Service) LaunchAtWifiPage(ctx context.Context, e *emptypb.Empty) (*emptypb.Empty, error) {
	cr := s.sharedObject.Chrome
	if cr == nil {
		return &emptypb.Empty{}, errors.New("Chrome has not been started")
	}

	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		condition := uiauto.New(tconn).Exists(nodewith.Name("Wi-Fi subpage back button").Ancestor(WindowFinder))
		if _, err := LaunchAtPageURL(ctx, tconn, cr, "networks?type=WiFi", condition); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to launch OS-Settings and navigate to Wi-Fi")
		}
		return &emptypb.Empty{}, nil
	})
}

// OpenNetworkDetailPage will open the OS Settings application and navigate
// to the detail page for the specified network.
func (s *Service) OpenNetworkDetailPage(ctx context.Context, req *pb.OpenNetworkDetailPageRequest) (*emptypb.Empty, error) {
	cr := s.sharedObject.Chrome
	if cr == nil {
		return &emptypb.Empty{}, errors.New("Chrome has not been started")
	}
	networkType, err := computeNetworkConfigNetworkType(req.NetworkType)
	if err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to determine network type")
	}

	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		if _, err = OpenNetworkDetailPage(ctx, tconn, cr, req.NetworkName, networkType); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to navigate to network detail page")
		}
		return &emptypb.Empty{}, nil
	})
}

// SetToggleOption clicks toggle option to enable or disable an option.
// It does nothing if the option is already expected.
func (s *Service) SetToggleOption(ctx context.Context, req *pb.SetToggleOptionRequest) (*emptypb.Empty, error) {
	cr := s.sharedObject.Chrome
	if cr == nil {
		return &emptypb.Empty{}, errors.New("Chrome has not been started")
	}

	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		osSettings := New(tconn)
		if err := osSettings.SetToggleOption(cr, req.ToggleOptionName, req.Enabled)(ctx); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to set toggle option")
		}
		return &emptypb.Empty{}, nil
	})
}

// WaitUntilToggleOption waits until the toggle option enabled or disabled.
func (s *Service) WaitUntilToggleOption(ctx context.Context, req *pb.WaitUntilToggleOptionRequest) (*emptypb.Empty, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		cr := s.sharedObject.Chrome
		if cr == nil {
			return &emptypb.Empty{}, errors.New("Chrome has not been started")
		}

		osSettings := New(tconn)
		if err := osSettings.WaitUntilToggleOption(cr, req.ToggleOptionName, req.Enabled)(ctx); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to check if toggle state is enabled")
		}

		return &emptypb.Empty{}, nil
	})
}

// Close will close the open OS Settings application.
func (s *Service) Close(ctx context.Context, e *emptypb.Empty) (*emptypb.Empty, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		if err := New(tconn).Close(ctx); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to close OS Settings")
		}
		return &emptypb.Empty{}, nil
	})
}

// EvalJSWithShadowPiercer executes javascript in Settings app web page.
func (s *Service) EvalJSWithShadowPiercer(ctx context.Context, req *pb.EvalJSWithShadowPiercerRequest) (*structpb.Value, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (_ *structpb.Value, retErr error) {
		cr := s.sharedObject.Chrome
		if cr == nil {
			return &structpb.Value{}, errors.New("Chrome has not been started")
		}

		var out interface{}
		osSettings := New(tconn)
		if err := osSettings.EvalJSWithShadowPiercer(ctx, cr, req.Expression, &out); err != nil {
			return &structpb.Value{}, errors.Wrap(err, "failed to execute javascript expression")
		}

		return structpb.NewValue(out)
	})
}

// AvailableWifiNetworks looks for all available WiFi networks in the WiFi detail page,
// returns a list of SSID of available WiFi networks.
// This RPC requires the WiFi detail page to be opened, an error will be returned if
// the page is not opened.
func (s *Service) AvailableWifiNetworks(ctx context.Context, e *emptypb.Empty) (*pb.AvailableWifiNetworksResponse, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (_ *pb.AvailableWifiNetworksResponse, retErr error) {
		// Extract the SSID from the name of the arrow button on the right of the generic container as
		// the content of the generic container contains multiple other information, hard to extract SSID from it.
		infos, err := New(tconn).NodesInfo(ctx, WifiNetworkDetailArrowButton)
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve the info of items in network list")
		}

		res := &pb.AvailableWifiNetworksResponse{
			Ssids: make([]string, 0, len(infos)),
		}
		for _, info := range infos {
			ss := WifiSubPageArrowButtonNameRegex.FindStringSubmatch(info.Name)
			if len(ss) < 2 {
				return nil, errors.Errorf("failed to extract SSID from %q", info.Name)
			}
			res.Ssids = append(res.Ssids, ss[1])
		}
		return res, nil
	})
}
