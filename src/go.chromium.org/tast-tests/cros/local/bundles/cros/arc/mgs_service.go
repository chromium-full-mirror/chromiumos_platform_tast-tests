/// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	arcpb "go.chromium.org/tast-tests/cros/services/cros/arc"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			arcpb.RegisterMGSServiceServer(srv, &MGSService{})
		},
	})
}

// MGSService implements tast.cros.arc.MGSService
type MGSService struct {
	cr *chrome.Chrome
}

// Enroll the device with the provided account credentials.
func (service *MGSService) Enroll(ctx context.Context, req *arcpb.EnrollRequest) (_ *empty.Empty, retErr error) {
	if service.cr != nil {
		return nil, errors.New("Device already enrolled")
	}

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ProdPolicy(),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: req.User, Pass: req.Pass}))
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Chrome")
	}
	service.cr = cr

	return &empty.Empty{}, nil
}

// RestartSession restarts the session.
func (service *MGSService) RestartSession(ctx context.Context, _ *empty.Empty) (_ *empty.Empty, retErr error) {
	if service.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	if err := chrome.PrepareForRestart(); err != nil {
		return nil, errors.Wrap(err, "failed to prepare Chrome for restart")
	}

	// Restart Chrome so MGS session starts automatically.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return nil, errors.Wrap(err, "failed to restart Chrome")
	}

	// Connection needs to be re-established after restart.
	service.cr.Reconnect(ctx)

	return &empty.Empty{}, nil
}

// WaitForPackages waits for the list of packages being installed in MGS.
func (service *MGSService) WaitForPackages(ctx context.Context, req *arcpb.WaitForPackagesRequest) (_ *empty.Empty, retErr error) {
	if service.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	const bootTimeout = 4 * time.Minute

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get outDir")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	a, err := arc.NewWithTimeout(ctx, outDir, bootTimeout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start ARC by user policy")
	}
	defer a.Close(cleanupCtx)

	if err := a.WaitForPackages(ctx, req.Packages); err != nil {
		return nil, errors.Wrap(err, "failed to force install packages")
	}

	if service.cr != nil {
		service.cr.Close(ctx)
		service.cr = nil
	}

	return &empty.Empty{}, nil
}
