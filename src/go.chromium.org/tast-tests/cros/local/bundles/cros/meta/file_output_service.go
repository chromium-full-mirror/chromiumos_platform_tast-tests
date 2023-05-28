// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/services/cros/meta"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testutil"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			meta.RegisterFileOutputServiceServer(srv, &FileOutputService{s: s})
		},
	})
}

// FileOutputService implements tast.cros.meta.FileOutputService.
type FileOutputService struct {
	s *testing.ServiceState
}

func (s *FileOutputService) SaveOutputFiles(ctx context.Context, req *meta.SaveOutputFilesRequest) (*empty.Empty, error) {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("output dir unavailable")
	}
	if err := testutil.WriteFiles(outDir, req.Files); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}
