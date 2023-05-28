// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var quickSettingsService Service
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			quickSettingsService = Service{sharedObject: common.SharedObjectsForServiceSingleton}
			pb.RegisterQuickSettingsServiceServer(srv, &quickSettingsService)
		},
	})
}

// Service implements tast.cros.chrome.uiauto.quicksettings.QuickSettingsService
type Service struct {
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
