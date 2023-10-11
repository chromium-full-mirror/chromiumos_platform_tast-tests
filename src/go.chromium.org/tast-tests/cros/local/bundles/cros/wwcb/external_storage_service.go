// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var externalStorageService ExternalStorageService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			externalStorageService = ExternalStorageService{sharedObject: common.SharedObjectsForServiceSingleton}
			wwcb.RegisterExternalStorageServiceServer(srv, &externalStorageService)
		},
		GuaranteeCompatibility: true,
	})
}

// ExternalStorageService implements tast.cros.wwcb.ExternalStorageService.
type ExternalStorageService struct {
	sharedObject *common.SharedObjectsForService
}

// EjectAll ejects all present USB Drives.
func (ds *ExternalStorageService) EjectAll(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}
	cleanupCtx := ctx

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch the Files App")
	}
	defer filesApp.Close(cleanupCtx)

	if err := filesApp.EjectAll()(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to eject filesapp")
	}

	return &empty.Empty{}, nil
}
