// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"google.golang.org/grpc"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/cryptohome"
	pb "go.chromium.org/tast-tests/cros/services/cros/camerabox"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterOrientationServiceServer(srv, &OrientationService{s: s})
		},
	})
}

type OrientationService struct {
	s *testing.ServiceState
}

// copyPreviewFrame copies the last preview frame image from download folder for debugging.
func (a *OrientationService) copyPreviewFrame(ctx context.Context, cr *chrome.Chrome) error {
	userPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get the cryptohome user path")
	}
	const frameFileName = "frame.png"
	framePath := filepath.Join(userPath, "MyFiles", "Downloads", frameFileName)
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get remote context dir")
	}
	if err := fsutil.CopyFile(framePath, filepath.Join(outDir, frameFileName)); err != nil {
		return errors.Wrap(err, "failed copy frame image to out dir")
	}
	return nil
}

func (a *OrientationService) CheckOrientation(ctx context.Context, req *pb.CheckOrientationRequest) (*pb.CheckOrientationResponse, error) {

	srv := httptest.NewServer(http.FileServer(http.Dir(req.DataPath)))
	defer srv.Close()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.ARCDisabled(),
		// Avoid the need to grant camera/microphone permissions.
		chrome.ExtraArgs("--use-fake-ui-for-media-stream"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}
	defer cr.Close(cleanupCtx)

	conn, err := cr.NewConn(ctx, srv.URL+"/wfc_orientation.html")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open wfc_orientation.html")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	if err := conn.Call(ctx, nil, "Tast.checkOrientation", req.Facing); err != nil {
		if err := a.copyPreviewFrame(cleanupCtx, cr); err != nil {
			testing.ContextLog(ctx, "Failed to copy last preview frame: ", err)
		}
		return &pb.CheckOrientationResponse{
			Result: pb.TestResult_TEST_RESULT_FAILED,
			Error:  err.Error(),
		}, nil
	}

	return &pb.CheckOrientationResponse{Result: pb.TestResult_TEST_RESULT_PASSED}, nil
}
