// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/common/hwsec"
	hwseclocal "chromiumos/tast/local/hwsec"
	pb "chromiumos/tast/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterTPMServiceServer(srv, &TPMService{s: s})
		},
	})
}

// TPMService implements tast.cros.firmware.TPMService.
type TPMService struct {
	s *testing.ServiceState
	// FullHelper has 3 helpers: AttestationHelper, CmdTPMClearHelper (which contains CmdHelper).
	helper *hwseclocal.FullHelperLocal
}

// NewHelper creates a new FullHelper for TPM.
func (ts *TPMService) NewHelper(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	helper, err := hwseclocal.NewFullHelper(ctx, hwseclocal.NewCmdRunner())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create hwsec full helper")
	}

	ts.helper = helper
	return &empty.Empty{}, nil
}

// CloseHelper closes existing helper.
func (ts *TPMService) CloseHelper(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if ts.helper == nil {
		return nil, errors.New("Helper not available")
	}
	ts.helper = nil
	return &empty.Empty{}, nil
}

// TPMVersion gets the current TPM version eg. "1.2" or "2.0". Requires a Helper to be created.
func (ts *TPMService) TPMVersion(ctx context.Context, req *empty.Empty) (*pb.TPMVersionResponse, error) {
	if ts.helper == nil {
		return nil, errors.New("Helper not available, create new tpm helper first")
	}
	tpmVer, err := ts.helper.CmdTPMClearHelper.CmdHelper.GetTPMVersion(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get TPM version, got: %v", tpmVer)
	}

	return &pb.TPMVersionResponse{Version: tpmVer}, nil
}

// StartDaemons starts TPM related daemons in reverse order of stopped daemons from low to high level.
func (ts *TPMService) StartDaemons(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if ts.helper == nil {
		return nil, errors.New("Helper not available, create new tpm helper first")
	}

	dc := ts.helper.CmdTPMClearHelper.CmdHelper.DaemonController()

	if err := dc.EnsureDaemons(ctx, hwsec.LowLevelTPMDaemons); err != nil {
		return nil, errors.Wrap(err, "failed to ensure low-level TPM daemons are running")
	}
	if err := dc.EnsureDaemons(ctx, hwsec.HighLevelTPMDaemons); err != nil {
		return nil, errors.Wrap(err, "failed to ensure high-level TPM daemons are running")
	}

	return &empty.Empty{}, nil
}

// StopDaemons stops TPM related daemons in specific order from high to low level.
func (ts *TPMService) StopDaemons(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if ts.helper == nil {
		return nil, errors.New("Helper not available, create new tpm helper first")
	}

	dc := ts.helper.CmdTPMClearHelper.CmdHelper.DaemonController()

	if err := dc.TryStopDaemons(ctx, hwsec.HighLevelTPMDaemons); err != nil {
		if _, startErr := ts.StartDaemons(ctx, &empty.Empty{}); startErr != nil {
			return nil, errors.Wrap(startErr, "failed to restart daemons after failing to stop high level TPM daemons")
		}
		return nil, errors.Wrap(err, "failed to try to stop high-level TPM daemons")
	}
	if err := dc.TryStopDaemons(ctx, hwsec.LowLevelTPMDaemons); err != nil {
		if _, startErr := ts.StartDaemons(ctx, &empty.Empty{}); startErr != nil {
			return nil, errors.Wrap(startErr, "failed to restart daemons after failing to stop low level TPM daemons")
		}
		return nil, errors.Wrap(err, "failed to try to stop low-level TPM daemons")
	}

	return &empty.Empty{}, nil
}

// GetAllVolatileFlags gets the current ST CLEAR flags. Requires a Helper to be created.
// This function also requires daemons to be stopped.
func (ts *TPMService) GetAllVolatileFlags(ctx context.Context, req *empty.Empty) (*pb.TPMFlags, error) {
	if ts.helper == nil {
		return nil, errors.New("Helper not available, create new tpm helper first")
	}
	flags, err := ts.helper.CmdTPMClearHelper.CmdHelper.GetAllVolatileFlags(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get volatile flags, got: %v", flags)
	}

	return &pb.TPMFlags{Flags: flags}, nil
}

// GetAllPermanentFlags gets the current permanent flags. Requires a Helper to be created.
// This function also requires daemons to be stopped.
func (ts *TPMService) GetAllPermanentFlags(ctx context.Context, req *empty.Empty) (*pb.TPMFlags, error) {
	if ts.helper == nil {
		return nil, errors.New("Helper not available, create new tpm helper first")
	}
	flags, err := ts.helper.CmdTPMClearHelper.CmdHelper.GetAllPermanentFlags(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get permanent flags, got: %v", flags)
	}

	return &pb.TPMFlags{Flags: flags}, nil
}

// GetSpacePermissions gets the current permission for a given space. Requires a Helper to be created.
// This function also requires daemons to be stopped.
func (ts *TPMService) GetSpacePermissions(ctx context.Context, req *pb.TPMSpacePermission) (*pb.TPMSpacePermission, error) {
	if ts.helper == nil {
		return nil, errors.New("Helper not available, create new tpm helper first")
	}
	permission, err := ts.helper.CmdTPMClearHelper.CmdHelper.GetSpacePermissions(ctx, req.Space)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get permission for space, got: %v", permission)
	}

	req.Permission = permission
	return req, nil
}
