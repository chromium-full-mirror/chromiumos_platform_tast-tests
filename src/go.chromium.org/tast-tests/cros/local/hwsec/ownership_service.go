// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	hwsecpb "go.chromium.org/tast-tests/cros/services/cros/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			hwsecpb.RegisterOwnershipServiceServer(srv, &OwnershipService{s})
		},
		// GuaranteeCompatibility allows tests outside ChromeOS to call this service.
		GuaranteeCompatibility: true,
	})
}

// OwnershipService implements tast.cros.hwsec.OwnershipService.
type OwnershipService struct {
	s *testing.ServiceState
}

// EnsureTPMIsReset calls the local EnsureTPMIsReset hwsec helpers.
func (*OwnershipService) EnsureTPMIsReset(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	testing.ContextLog(ctx, "Requesting a local TPM reset")

	cmdRunner := NewCmdRunner()
	helper, err := NewHelper(cmdRunner)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create local helper")
	}

	if err := helper.EnsureTPMIsReset(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reset TPMd")
	}

	return &empty.Empty{}, nil
}

// EnsureTPMAndSystemStateAreReset calls the local EnsureTPMAndSystemStateAreReset hwsec helpers.
func (*OwnershipService) EnsureTPMAndSystemStateAreReset(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	testing.ContextLog(ctx, "Requesting a local TPM and state reset")

	cmdRunner := NewCmdRunner()
	helper, err := NewHelper(cmdRunner)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create local helper")
	}

	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reset TPMd")
	}

	return &empty.Empty{}, nil
}
