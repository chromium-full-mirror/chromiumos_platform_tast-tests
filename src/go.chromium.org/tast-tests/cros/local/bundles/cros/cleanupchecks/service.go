// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cleanupchecks

import (
	"context"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/cleanupchecks"
	cs "go.chromium.org/tast-tests/cros/services/cros/cleanupchecks"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			cs.RegisterCleanUpChecksServiceServer(srv, &Service{})
		},
	})
}

// Service implements tast.cros.cleanupchecks.CleanUpChecksService.
type Service struct {
}

// RecordStateBeforeTestFromDUT executes clean up checks on the DUT.
func (service *Service) RecordStateBeforeTestFromDUT(ctx context.Context, req *cs.RecordStateBeforeTestFromDUTRequest) (*cs.RecordStateBeforeTestFromDUTResponse, error) {
	return cleanupchecks.RecordStateBeforeTest(ctx, req)
}

// ExecuteCleanUpChecksOnDUT executes clean up checks on the DUT.
func (service *Service) ExecuteCleanUpChecksOnDUT(ctx context.Context, req *cs.ExecuteCleanUpChecksOnDUTRequest) (*cs.ExecuteCleanUpChecksOnDUTResponse, error) {
	return cleanupchecks.ExecuteCleanUpChecks(ctx, req)
}
