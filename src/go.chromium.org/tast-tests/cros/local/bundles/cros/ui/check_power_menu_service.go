// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/procutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterPowerMenuServiceServer(srv, &PowerMenuService{s: s})
		},
	})
}

// PowerMenuService implements tast.cros.ui.PowerMenuService.
type PowerMenuService struct {
	s              *testing.ServiceState
	cr             *chrome.Chrome
	loginRequested bool
	tconn          *chrome.TestConn
}

func (p *PowerMenuService) NewChrome(ctx context.Context, req *pb.NewChromeRequest) (*empty.Empty, error) {
	if p.cr != nil {
		return nil, errors.New("Chrome already available")
	}

	oldproc, err := ashproc.Root()
	if err != nil {
		return nil, errors.Wrap(err, "error getting Chrome root PID")
	}

	p.loginRequested = req.Login
	if p.loginRequested {
		p.cr, err = chrome.New(ctx, chrome.Region("us"))
	} else {
		p.cr, err = chrome.New(ctx, chrome.Region("us"), chrome.KeepState(), chrome.NoLogin(), chrome.LoadSigninProfileExtension(req.Key))
	}
	if err != nil {
		return nil, err
	}

	// Make sure older chrome is gone.
	if err := procutil.WaitForTerminated(ctx, oldproc, 30*time.Second); err != nil {
		return nil, errors.Wrap(err, "chrome is not terminated")
	}

	// Then, wait for the new chrome processes.
	if _, err := ashproc.WaitForRoot(ctx, 30*time.Second); err != nil {
		return nil, errors.Wrap(err, "chrome is not restarted")
	}

	return &empty.Empty{}, nil
}

func (p *PowerMenuService) CloseChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if p.cr == nil {
		return nil, errors.New("Chrome not available")
	}
	err := p.cr.Close(ctx)
	p.cr = nil
	return &empty.Empty{}, err
}

func (p *PowerMenuService) PowerMenuPresent(ctx context.Context, req *empty.Empty) (*pb.PowerMenuPresentResponse, error) {
	if p.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	var err error
	if p.loginRequested {
		p.tconn, err = p.cr.TestAPIConn(ctx)
	} else {
		p.tconn, err = p.cr.SigninProfileTestAPIConn(ctx)
	}
	if err != nil {
		return nil, err
	}

	// TODO(b:399557656): remove once cause is understood
	// OS builds after 16183.0.0 seem to break the UI automation on the lock
	// screen, where it only updates when we dump the entire tree. Otherwise,
	// automation fails to find the power menu on the lock screen even if it
	// is actually visible.
	_, err = uiauto.RootDebugInfo(ctx, p.tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to dump UI tree")
	}

	// Check if the power menu is displayed
	finder := nodewith.ClassName("PowerButtonMenuView").Onscreen().First()
	exists, err := uiauto.New(p.tconn).IsNodeFound(ctx, finder)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find power menu")
	}

	return &pb.PowerMenuPresentResponse{IsMenuPresent: exists}, nil
}

// PowerMenuItem reads from the power menu.
// Checking PowerMenuPresent is required prior to calling PowerMenuItem.
func (p *PowerMenuService) PowerMenuItem(ctx context.Context, req *empty.Empty) (*pb.PowerMenuItemResponse, error) {
	if p.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	if p.tconn == nil {
		return nil, errors.New("Test API connection not available")
	}

	ui := uiauto.New(p.tconn)
	menu := nodewith.ClassName("PowerButtonMenuItemView")
	menuItems, err := ui.NodesInfo(ctx, menu)
	if err != nil {
		return nil, err
	}

	var itemsName []string
	for _, val := range menuItems {
		itemsName = append(itemsName, val.Name)
	}

	return &pb.PowerMenuItemResponse{MenuItems: itemsName}, nil
}

// Lock locks the screen.
func (p *PowerMenuService) Lock(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if p.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	tconn, err := p.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	// Lock the screen.
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to lock the screen")
	}

	return &empty.Empty{}, nil
}
