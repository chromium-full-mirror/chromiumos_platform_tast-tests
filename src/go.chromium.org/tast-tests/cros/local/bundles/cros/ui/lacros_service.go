// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterLacrosServiceServer(srv,
				&LacrosService{sharedObject: common.SharedObjectsForServiceSingleton})
		},
		GuaranteeCompatibility: true,
	})
}

// LacrosService implements tast.cros.ui.LacrosService.
type LacrosService struct {
	sharedObject *common.SharedObjectsForService
	lacros       *lacros.Lacros
}

// Launch instantiates svc.lacros by calling lacros.Launch.
func (svc *LacrosService) Launch(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	return svc.instantiateLacrosWithFn(ctx, lacros.Launch)
}

// Connect instantiates svc.lacros by calling lacros.Connect.
func (svc *LacrosService) Connect(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	return svc.instantiateLacrosWithFn(ctx, lacros.Connect)
}

// LaunchWithURL instantiates svc.lacros by calling lacros.LaunchWithURL.
func (svc *LacrosService) LaunchWithURL(ctx context.Context, req *pb.LaunchWithURLRequest) (*empty.Empty, error) {
	return svc.instantiateLacrosWithFn(ctx, func(ctx context.Context, tconn *chrome.TestConn) (*lacros.Lacros, error) {
		l, conn, err := lacros.LaunchWithURL(ctx, tconn, req.Url)
		// conn is returned but not used.
		// As suggested by the function, it needs to be closed explicitly by caller.
		if err == nil {
			if err := conn.Close(); err != nil {
				testing.ContextLog(ctx, "Failed to close conn: ", err)
			}
		}

		return l, err
	})
}

// Close releases svc.lacros and calls lacros.Close.
func (svc *LacrosService) Close(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	l := svc.lacros
	svc.lacros = nil
	svc.sharedObject.LacrosBrowser = nil

	if l == nil {
		return nil, errors.New("Lacros is not instantiated")
	}

	if err := l.Close(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close Lacros: ", err)
		return nil, errors.Wrap(err, "failed to close Lacros")
	}

	return &empty.Empty{}, nil
}

// instantiateLacrosWithFn instantiates tast Lacros instance by calling the function passed as fn.
func (svc *LacrosService) instantiateLacrosWithFn(ctx context.Context, fn func(context.Context, *chrome.TestConn) (*lacros.Lacros, error)) (*empty.Empty, error) {
	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	cr := svc.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}

	tconn, err := svc.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	l, err := fn(ctx, tconn)
	if err != nil {
		return nil, err
	}

	svc.lacros = l
	svc.sharedObject.LacrosBrowser = l.Browser()
	return &empty.Empty{}, nil
}
