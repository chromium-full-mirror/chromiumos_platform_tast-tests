// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/upstart"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
)

func init() {
	var chromeUIService ChromeUIService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			chromeUIService = ChromeUIService{sharedObject: common.SharedObjectsForServiceSingleton}
			pb.RegisterChromeUIServiceServer(srv, &chromeUIService)
		},
	})
}

// ChromeUIService implements the methods defined in ChromeUIServiceServer.
type ChromeUIService struct {
	sharedObject *common.SharedObjectsForService
}

// EnsureLoginScreen emulates log out, and ensures login screen.
func (c *ChromeUIService) EnsureLoginScreen(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to restart ui job")
	}
	return &empty.Empty{}, nil
}

// DumpUITree dumps the UI tree to the context output directory of the test.
func (c *ChromeUIService) DumpUITree(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cr := c.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}

	contextOutDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get the context output directory")
	}

	return common.UseTconn(ctx, c.sharedObject, func(tconn *chrome.TestConn) (*empty.Empty, error) {
		faillog.DumpUITree(ctx, contextOutDir, tconn)
		return &empty.Empty{}, nil
	})
}

// DumpUITreeWithScreenshotToFile dumps the UI tree with screenshot to the file.
func (c *ChromeUIService) DumpUITreeWithScreenshotToFile(ctx context.Context, req *pb.DumpUITreeWithScreenshotToFileRequest) (*empty.Empty, error) {
	cr := c.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}

	contextOutDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get the context output directory")
	}

	return common.UseTconn(ctx, c.sharedObject, func(tconn *chrome.TestConn) (*empty.Empty, error) {
		faillog.DumpUITreeWithScreenshotWithTestAPIOnError(ctx, contextOutDir, func() bool { return true }, tconn, req.FilePrefix)
		return &empty.Empty{}, nil
	})
}

// WaitForWelcomeScreen waits for welcome screen to be shown in OOBE.
func (c *ChromeUIService) WaitForWelcomeScreen(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cr := c.sharedObject.Chrome
	if cr == nil {
		return &empty.Empty{}, errors.New("Chrome has not been started")
	}

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		return &empty.Empty{}, errors.New("failed to create OOBE connection")
	}
	defer oobeConn.Close()

	// TODO(b/246649651): Separate this logic into a helper method for other OOBE
	// tests to use.
	// Wait for the welcome screen to be shown.
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		return &empty.Empty{}, errors.New("failed to wait for the welcome screen to be visible")
	}
	return &empty.Empty{}, nil
}
