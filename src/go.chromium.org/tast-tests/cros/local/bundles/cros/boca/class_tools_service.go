// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package boca

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/services/cros/boca"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			boca.RegisterClassToolsServiceServer(srv, &ClassToolsService{s: s})
		},
	})
}

// ClassToolsService implements tast.cros.boca.ClassToolsService.
type ClassToolsService struct {
	s     *testing.ServiceState
	cr    *chrome.Chrome
	tconn *chrome.TestConn
}

// NewChromeLogin logs into Chrome with Boca and BocaConsumer flags enabled.
func (cts *ClassToolsService) NewChromeLogin(ctx context.Context, req *boca.CrOSLoginRequest) (*empty.Empty, error) {

	if cts.cr != nil {
		return nil, errors.New("Chrome already available")
	}
	bocaServiceOpts := []chrome.Option{
		chrome.EnableFeatures(req.EnabledFlags),
	}

	if req.Username != "" {
		bocaServiceOpts = append(bocaServiceOpts, chrome.GAIALogin(chrome.Creds{User: req.Username, Pass: req.Password}))
	}

	cr, err := chrome.New(ctx, bocaServiceOpts...)

	if err != nil {
		testing.ContextLog(ctx, "Failed to start Chrome")
		return nil, err
	}
	cts.cr = cr
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get a connection to the Test Extension")
		return nil, err
	}
	cts.tconn = tconn
	return &empty.Empty{}, nil
}

// CloseChrome closes all surfaces and Chrome.
// This will likely be called in a defer in remote tests instead of called explicitly. So log everything that fails to aid debugging later.
func (cts *ClassToolsService) CloseChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if cts.cr == nil {
		testing.ContextLog(ctx, "Chrome not available")
		return nil, errors.New("Chrome not available")
	}

	err := cts.cr.Close(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Faied to close Chrome: ", err)
	}
	cts.cr = nil
	return &empty.Empty{}, err
}

func (cts *ClassToolsService) VerifySessionStart(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {

	cleanupCtx := ctx

	tconn := cts.tconn
	ui := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to initialize keybaord: ", err)
	}
	defer kb.Close(cleanupCtx)

	if err := launcher.SearchAndLaunchWithQuery(tconn, kb, "school tools", "School Tools")(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to find School tools app in the launcher: ", err)
		return &empty.Empty{}, err
	}

	selectClassDialog := nodewith.Role(role.Button).HasClass("filled-tonal").First()
	if err := ui.LeftClick(selectClassDialog)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to open select class dialog: ", err)
		return &empty.Empty{}, err
	}

	selectClassButton := nodewith.Role(role.Button).Name("Select").HasClass("button").First()
	if err := ui.LeftClick(selectClassButton)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to select a class: ", err)
		return &empty.Empty{}, err
	}

	confirmClassButton := nodewith.Role(role.Button).Name("Confirm").HasClass("button")
	if err := ui.LeftClick(confirmClassButton)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to select students: ", err)
		return &empty.Empty{}, err
	}

	startClassButton := nodewith.Role(role.Button).Name("Start 1 hour class").HasClass("button")
	if err := ui.LeftClick(startClassButton)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to start class: ", err)
		return &empty.Empty{}, err
	}

	return &empty.Empty{}, err
}

func (cts *ClassToolsService) VerifyStudentSession(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cleanupCtx := ctx
	tconn := cts.tconn
	ui := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to initialize keybaord: ", err)
	}
	defer kb.Close(cleanupCtx)

	if err :=
		launcher.SearchAndLaunchWithQuery(tconn, kb, "school tools", "School Tools")(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to find School tools app in the launcher: ", err)
		return &empty.Empty{}, err
	}

	subtitle := nodewith.Name("You are in tast teacher's class").Role(role.StaticText)

	err2 := ui.Exists(subtitle)(ctx)
	if err2 != nil {
		testing.ContextLog(ctx, "Failed to verify is student has session started: ", err2)
		return &empty.Empty{}, err2
	}

	return &empty.Empty{}, err
}

func (cts *ClassToolsService) EndSession(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {

	tconn := cts.tconn
	ui := uiauto.New(tconn)

	endClassDialog := nodewith.Role(role.Button).Name("End").HasClass("button")
	if err := ui.LeftClick(endClassDialog)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to open end class dialog: ", err)
	}

	endClass := nodewith.Role(role.Button).Name("End class").HasClass("button")
	err := ui.LeftClick(endClass)(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to end class: ", err)
	}

	return &empty.Empty{}, err
}
