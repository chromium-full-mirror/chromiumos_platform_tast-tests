// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	pb "go.chromium.org/tast-tests/cros/services/cros/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterVMServiceServer(srv, &Service{s: s})
		},
	})
}

// Service implements tast.cros.vm.VmService.
type Service struct {
	s *testing.ServiceState
}

// RunARC runs ARC on the DUT.
func (c *Service) RunARC(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cr, err := chrome.New(ctx, chrome.ARCEnabled())
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Chrome")
	}
	defer func() {
		if err := cr.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close Chrome")
		}
	}()

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.Wrap(err, "failed to get output dir")
	}

	a, err := arc.New(ctx, outDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start ARC")
	}
	defer func() {
		a.Close(ctx)
	}()
	return &empty.Empty{}, nil
}
