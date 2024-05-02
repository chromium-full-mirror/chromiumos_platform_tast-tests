// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const defaultAppLaunchTimeout = 60
const defaultAppCloseTimeout = 10

func init() {
	var svc service
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			svc = service{sharedObject: common.SharedObjectsForServiceSingleton}
			pb.RegisterAppsServiceServer(srv, &svc)
		},
		GuaranteeCompatibility: true,
	})
}

// Service implements tast.cros.apps.AppsService.
type service struct {
	sharedObject *common.SharedObjectsForService
}

// LaunchApp launches an app.
func (svc *service) LaunchApp(ctx context.Context, req *pb.LaunchAppRequest) (*empty.Empty, error) {
	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = defaultAppLaunchTimeout
	}
	return common.UseTconn(ctx, svc.sharedObject, func(tconn *chrome.TestConn) (*empty.Empty, error) {
		appID, err := InstalledAppID(ctx, tconn, func(app *ash.ChromeApp) bool {
			return app.Name == req.AppName
		}, &testing.PollOptions{Timeout: time.Duration(req.TimeoutSecs) * time.Second})
		if err != nil {
			return nil, err
		}
		if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.launchApp)`, appID); err != nil {
			return nil, errors.Wrapf(err, "failed to launch app %s", req.AppName)
		}
		if err := ash.WaitForApp(ctx, tconn, appID, time.Duration(req.TimeoutSecs)*time.Second); err != nil {
			return nil, errors.Wrapf(err, "app %s never opened", req.AppName)
		}
		return &empty.Empty{}, nil
	})
}

// CloseApp closes an app.
func (svc *service) CloseApp(ctx context.Context, req *pb.CloseAppRequest) (*empty.Empty, error) {
	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = defaultAppCloseTimeout
	}
	return common.UseTconn(ctx, svc.sharedObject, func(tconn *chrome.TestConn) (*empty.Empty, error) {
		appID, err := InstalledAppID(ctx, tconn, func(app *ash.ChromeApp) bool {
			return app.Name == req.AppName
		}, &testing.PollOptions{Timeout: time.Duration(req.TimeoutSecs) * time.Second})
		if err != nil {
			return nil, err
		}

		isAppRunning, err := ash.AppRunning(ctx, tconn, appID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check app %s running", req.AppName)
		} else if !isAppRunning {
			return nil, errors.Wrapf(err, "app %s is not running", req.AppName)
		}

		if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.closeApp)`, appID); err != nil {
			return nil, errors.Wrapf(err, "failed to close app %s", req.AppName)
		}
		if err := ash.WaitForAppClosed(ctx, tconn, appID); err != nil {
			return nil, errors.Wrapf(err, "app %s is not closed", req.AppName)
		}
		return &empty.Empty{}, nil
	})
}

// GetPrimaryBrowser returns the App used for the primary browser.
func (svc *service) GetPrimaryBrowser(ctx context.Context, req *empty.Empty) (*pb.App, error) {
	return common.UseTconn(ctx, svc.sharedObject, func(tconn *chrome.TestConn) (*pb.App, error) {
		app, err := PrimaryBrowser(ctx, tconn)
		if err != nil {
			return nil, err
		}
		return &pb.App{Id: app.ID, Name: app.Name}, nil
	})
}

// LaunchPrimaryBrowser launches the primary browser and returns the App launched.
func (svc *service) LaunchPrimaryBrowser(ctx context.Context, req *empty.Empty) (*pb.App, error) {
	app, err := svc.GetPrimaryBrowser(ctx, req)
	if err != nil {
		return app, err
	}
	_, err = svc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: app.Name})
	return app, err
}
