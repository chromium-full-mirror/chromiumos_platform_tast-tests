// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var quickSettingsService Service
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			quickSettingsService = Service{sharedObject: common.SharedObjectsForServiceSingleton, serviceState: s}
			pb.RegisterQuickSettingsServiceServer(srv, &quickSettingsService)
		},
	})
}

// Service implements tast.cros.chrome.uiauto.quicksettings.QuickSettingsService
type Service struct {
	serviceState *testing.ServiceState
	sharedObject *common.SharedObjectsForService
}

// NavigateToNetworkDetailedView will navigate to the detailed Network view
// within the Quick Settings. This is safe to call even when the Quick Settings
// are already open.
func (s *Service) NavigateToNetworkDetailedView(ctx context.Context, e *empty.Empty) (*empty.Empty, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		return &emptypb.Empty{}, NavigateToNetworkDetailedView(ctx, tconn)
	})
}

// Hide hides the Quick Settings.
func (s *Service) Hide(ctx context.Context, e *empty.Empty) (*empty.Empty, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		return &emptypb.Empty{}, Hide(ctx, tconn)
	})
}

// ToggleOption clicks the specified toggle button to enable or disable an option.
// It does nothing if the state of the toggle button is already as expected.
func (s *Service) ToggleOption(ctx context.Context, req *pb.ToggleOptionRequest) (*empty.Empty, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*emptypb.Empty, error) {
		cleanup, err := ensureVisible(ctx, tconn)
		if err != nil {
			return &emptypb.Empty{}, err
		}
		defer cleanup(ctx)

		var toggleButton *nodewith.Finder
		switch req.GetToggleButton() {
		case pb.ToggleOptionRequest_Wifi:
			toggleButton = NetworkDetailedViewWifiToggleButton

			if err := NavigateToNetworkDetailedView(ctx, tconn); err != nil {
				return &emptypb.Empty{}, errors.Wrap(err, "failed to navigate to network detailed view")
			}
		default:
			return nil, errors.New("not supported toggle option")
		}

		return &emptypb.Empty{}, ToggleOption(ctx, tconn, toggleButton, req.GetEnabled())
	})
}

// AvailableWifiNetworks looks for all available WiFi networks in the network detailed view,
// returns a list of SSID of available WiFi networks.
// This RPC requires the network detailed view to be opened, an error will be returned if
// the view is not presented.
func (s *Service) AvailableWifiNetworks(ctx context.Context, e *empty.Empty) (*pb.AvailableWifiNetworksResponse, error) {
	return common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (_ *pb.AvailableWifiNetworksResponse, retErr error) {
		infos, err := uiauto.New(tconn).NodesInfo(ctx, NetworkListItemView)
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve the info of items in network list")
		}

		res := &pb.AvailableWifiNetworksResponse{
			Ssids: make([]string, 0, len(infos)),
		}
		for _, info := range infos {
			if strings.Contains(info.Name, "Ethernet") {
				continue
			}

			// The name of a connected network in Wi-Fi network list is a string with prefix: "Open settings for "
			name := strings.TrimPrefix(info.Name, "Open settings for ")
			// The name of a not connected network in Wi-Fi network list is a string with prefix: "Connect to "
			name = strings.TrimPrefix(name, "Connect to ")
			// The remaining part should be the SSID alone.
			res.Ssids = append(res.Ssids, name)
		}
		return res, nil
	})
}
