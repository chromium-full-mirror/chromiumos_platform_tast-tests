// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nebraska

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	aupb "chromiumos/tast/services/cros/autoupdate"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			aupb.RegisterNebraskaServiceServer(srv, &Service{s: s})
		},
	})
}

// Service implements tast.cros.policy.NebraskaService.
type Service struct {
	s *testing.ServiceState

	instance *Nebraska
	tmpDir   string
	logPath  string
}

// CreateTempDir creates a temporary directory that is used by Nebraska.
func (n *Service) CreateTempDir(ctx context.Context, req *empty.Empty) (*aupb.CreateTempDirResponse, error) {
	dir, err := ioutil.TempDir("", "nebraska")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp dir")
	}
	n.tmpDir = dir

	return &aupb.CreateTempDirResponse{Path: dir}, nil
}

// Start starts a Nebraska service instance with the given parameters.
func (n *Service) Start(ctx context.Context, req *aupb.StartRequest) (*aupb.StartResponse, error) {
	logPath := filepath.Join(n.tmpDir, "nebraska.log")

	// Collect the arguments.
	args := []string{
		"--log-file", logPath,
	}

	if req.Port != "" {
		testing.ContextLog(ctx, "Adding port to arguments")
		args = append(args, "--port", req.Port)
	}

	if req.Update != nil {
		testing.ContextLog(ctx, "Adding update to arguments")
		args = append(args,
			"--update-metadata", req.Update.MetadataFolder,
			"--update-payloads-address", req.Update.Address,
		)
	}

	// Start the Nebraska service.
	instance, err := Start(n.s.ServiceContext(), n.tmpDir, args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Nebraska")
	}
	n.instance = instance

	if req.Port != "" && req.Port != strconv.Itoa(instance.Port) {
		if err := instance.Stop(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to stop Nebraska")
		}
		return nil, errors.Errorf("Nebraska started with wrong port; want %s, got %d", req.Port, instance.Port)
	}

	return &aupb.StartResponse{
		Port:    strconv.Itoa(n.instance.Port),
		LogPath: logPath,
	}, nil
}

// Stop gracefully stops the previously started Nebraska instance.
func (n *Service) Stop(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := n.instance.Stop(ctx); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// RemoveTempDir removes the temporary directory that was created for Nebraska.
func (n *Service) RemoveTempDir(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if n.tmpDir == "" {
		testing.ContextLog(ctx, "No temp diretory to remove")
		return &empty.Empty{}, nil
	}

	testing.ContextLog(ctx, "Deleting temp dir")
	if err := os.RemoveAll(n.tmpDir); err != nil {
		testing.ContextLogf(ctx, "Failed to delete %s: %v", n.tmpDir, err)
	}

	n.tmpDir = ""

	return &empty.Empty{}, nil
}
