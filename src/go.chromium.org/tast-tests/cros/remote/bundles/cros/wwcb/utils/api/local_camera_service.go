// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package api

import (
	"context"

	"google.golang.org/grpc"

	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/errors"
)

// CameraService wraps go.chromium.org/chromiumos/config/go/test/lab/api/passport.CameraServiceServer
//
// We use this interface as an intermediate so when we have a local implementation
// of passport.CameraServiceServer, we don't break Tast when go.chromium.org/chromiumos/config
// is auto rolled with a new change.
type CameraService interface {
	// GetCameras probes all connected cameras to the host device.
	GetCameras(ctx context.Context, req *passport.GetCamerasRequest, opts ...grpc.CallOption) (*passport.GetCamerasResponse, error)
	// GetAveragePixel gets the average pixel color detected by the specified camera.
	GetAveragePixel(ctx context.Context, req *passport.GetAveragePixelRequest, opts ...grpc.CallOption) (*passport.GetAveragePixelResponse, error)
}

// localCameraService implements api/passport.CameraService by wrapping cameras controllers to interact
// with cameras connected directly to the same host running the test.
type localCameraService struct {
}

// NewLocalCameraService creates a camera service for manipulating cameras connected locally to
// the host without running a separate gRPC service.
func NewLocalCameraService(ctx context.Context) (CameraService, error) {
	if err := utils.InitWebcam(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to initialize cameras on host")
	}
	return &localCameraService{}, nil
}

// GetCameras probes all connected cameras to the host device.
func (s *localCameraService) GetCameras(ctx context.Context, req *passport.GetCamerasRequest, opts ...grpc.CallOption) (*passport.GetCamerasResponse, error) {
	var cameras []*passport.Camera
	for _, cam := range utils.CamerasOnline() {
		cameras = append(cameras, &passport.Camera{Id: cam})
	}
	return &passport.GetCamerasResponse{Cameras: cameras}, nil
}

// GetAveragePixel gets the average pixel color detected by the specified camera.
func (s *localCameraService) GetAveragePixel(ctx context.Context, req *passport.GetAveragePixelRequest, opts ...grpc.CallOption) (*passport.GetAveragePixelResponse, error) {
	pixel, frame, err := utils.GetAvgPixelFromWebcam(ctx, req.GetDeviceId())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get average pixel from webcam")
	}

	return &passport.GetAveragePixelResponse{
		Pixel: pixel,
		Frame: frame,
	}, nil
}
