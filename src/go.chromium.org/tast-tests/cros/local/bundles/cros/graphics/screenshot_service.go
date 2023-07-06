// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pb "go.chromium.org/tast-tests/cros/services/cros/graphics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			graphics.RegisterScreenshotServiceServer(srv, &ScreenshotService{s: s})
		},
	})
}

// ScreenshotService implements tast.cros.graphics.ScreenshotService.
type ScreenshotService struct {
	s *testing.ServiceState
}

// CaptureScreenAndDelete captures a temporary screenshot, and deletes it immediately.
func (s *ScreenshotService) CaptureScreenAndDelete(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {

	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		return nil, errors.New("output directory unavailable")
	}
	path := filepath.Join(dir, "screenshotTest.png")
	defer os.Remove(path)
	if err := screenshot.CaptureWithStderr(ctx, path); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// CaptureScreenshot captures a screenshot and saves it in the output directory of the test under filePrefix.png.
func (s *ScreenshotService) CaptureScreenshot(ctx context.Context, req *pb.CaptureScreenshotRequest) (*empty.Empty, error) {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		return nil, errors.New("output directory unavailable")
	}
	path := filepath.Join(dir, req.FilePrefix+".png")

	testing.ContextLog(ctx, "Capturing screenshot at ", path)

	if err := screenshot.Capture(ctx, path); err != nil {
		//TODO(b/290188177): Move the retry to the screenshot package.
		testing.ContextLog(ctx, "Capturing screenshot failed: ", err)
		testing.ContextLog(ctx, "Trying to turn on display")

		if turnErr := power.TurnOnDisplay(ctx); turnErr != nil {
			testing.ContextLog(ctx, "Failed to turn on display: ", turnErr)
			return nil, err
		}

		if err := screenshot.Capture(ctx, path); err != nil {
			return nil, err
		}
	}
	return &empty.Empty{}, nil
}
