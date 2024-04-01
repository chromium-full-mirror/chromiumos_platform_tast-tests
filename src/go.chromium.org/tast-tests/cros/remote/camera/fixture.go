// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package camera contains camera-related utility functions for remote tests.
package camera

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/camera"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	rebootTimeout = 10 * time.Minute
	checkTimeout  = 3 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.CameraEnumerated,
		Desc: "Reboot if built-in cameras are not enumerated",
		Contacts: []string{
			"chromeos-camera-eng@google.com",
			"hidenorik@google.com"},
		Impl:            &cameraFixture{},
		SetUpTimeout:    rebootTimeout,
		TearDownTimeout: rebootTimeout,
		ResetTimeout:    checkTimeout,
		ServiceDeps:     []string{"tast.cros.camera.EnumService"},
	})
}

type cameraFixture struct {
	d *dut.DUT
	h *testing.RPCHint
}

func (f *cameraFixture) checkBuiltinCameraEnumerated(ctx context.Context) error {
	cl, err := rpc.Dial(ctx, f.d, f.h)
	if err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}
	defer cl.Close(ctx)

	bc := pb.NewEnumServiceClient(cl.Conn)
	_, err = bc.CheckBuiltinCamera(ctx, &emptypb.Empty{})
	return err
}

func (f *cameraFixture) ensureBuiltinCameraEnumerated(ctx context.Context) error {
	if err := f.checkBuiltinCameraEnumerated(ctx); err != nil {
		testing.ContextLog(ctx, "rebooting to restore camera, due to: ", err)
		if err := f.d.Reboot(ctx); err != nil {
			return errors.Wrap(err, "failed to reboot")
		}
		return f.checkBuiltinCameraEnumerated(ctx)
	}
	return nil
}

func (f *cameraFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// TODO(b/324145508): We now assume that all the errors can be fixed by waiting.
	// Monitor the results of the PlatformServiceSmoke test that uses this fixture.
	// Delete this comment once it proves to be enough. Ref: b/239013478.
	if err := s.DUT().Health(ctx); err != nil {
		s.Log("Failed connection check: ", err)

		waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if err := s.DUT().WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect: ", err)
		}
		s.Log("Successfully reconnected to DUT")
	}

	f.d = s.DUT()
	f.h = s.RPCHint()

	if err := f.ensureBuiltinCameraEnumerated(ctx); err != nil {
		s.Fatal("Failed to recover camera: ", err)
	}

	return nil
}

func (f *cameraFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.ensureBuiltinCameraEnumerated(ctx); err != nil {
		s.Fatal("Failed to recover camera: ", err)
	}
}

func (f *cameraFixture) Reset(ctx context.Context) error {
	return f.checkBuiltinCameraEnumerated(ctx)
}

func (f *cameraFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *cameraFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
