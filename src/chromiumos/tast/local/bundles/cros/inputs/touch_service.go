// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/common"
	"chromiumos/tast/local/input"
	pb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var touchService TouchService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			touchService = TouchService{sharedObject: common.SharedObjectsForServiceSingleton}
			pb.RegisterTouchServiceServer(srv, &touchService)
		},
		GuaranteeCompatibility: true,
	})
}

// TouchService implements tast.cros.inputs.TouchService.
type TouchService struct {
	sharedObject *common.SharedObjectsForService
	mutex        sync.Mutex
}

// Swipe performs a swipe movement with a user defined number of touches. The
// touches are separated by (dx, dy). For example, in a 3-touch swipe, the
// touches begin at (x0, y0), (x0+dx, y0+dy), (x0+2dx, y0+2dy).
func (svc *TouchService) Swipe(ctx context.Context, req *pb.SwipeRequest) (*empty.Empty, error) {
	svc.mutex.Lock()
	defer svc.mutex.Unlock()

	if req.Touches <= 0 {
		return nil, errors.New("number of touches has to be larger than 0")
	}

	// If TimeMilliSeconds is less than 5 milliseconds, a default value of 5 milliseconds
	// will be used instead.
	if req.TimeMilliSeconds == 0 {
		req.TimeMilliSeconds = 5
	}

	// Create touch screen and multi touch writer.
	tsw, err := input.Touchscreen(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a touchscreen device")
	}
	defer tsw.Close(ctx)
	tw, err := tsw.NewMultiTouchWriter(int(req.Touches))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a touch writer")
	}
	defer tw.Close()

	// The touchscreen dimension might not be the same as the display dimension.
	// In fact, might be even up to 4x bigger.
	// The input coordinates have to be scaled accordingly.
	// Get the display dimension.
	displayWidth, displayHeight, err := svc.getDisplayDimensions(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get display dimension")
	}

	// Get the touchscreen dimension.
	touchWidth := int(tsw.Width())
	touchHeight := int(tsw.Height())

	// Scale X and Y coordinates independently.
	x0 := int(req.X0)
	y0 := int(req.Y0)
	x1 := int(req.X1)
	y1 := int(req.Y1)
	dx := int(req.Dx)
	dy := int(req.Dy)

	if touchWidth != displayWidth {
		x0 = x0 * touchWidth / displayWidth
		x1 = x1 * touchWidth / displayWidth
		dx = dx * touchWidth / displayWidth
	}
	if touchHeight != displayHeight {
		y0 = y0 * touchHeight / displayHeight
		y1 = y1 * touchHeight / displayHeight
		dy = dy * touchHeight / displayHeight
	}

	testing.ContextLogf(ctx, "Display dimemsion: (%d, %d). Touchscreen dimension: (%d, %d)", displayWidth, displayHeight, touchWidth, touchHeight)
	testing.ContextLogf(ctx, "Scale inputs (x0, y0 ,x1 ,y1 ,dx ,dy) from (%d, %d, %d, %d, %d, %d) to (%d, %d, %d, %d, %d, %d)",
		req.X0, req.Y0, req.X1, req.Y1, req.Dx, req.Dy, x0, y0, x1, y1, dx, dy)

	if err != tw.Swipe(ctx,
		input.TouchCoord(x0),
		input.TouchCoord(y0),
		input.TouchCoord(x1),
		input.TouchCoord(y1),
		input.TouchCoord(dx),
		input.TouchCoord(dy),
		int(req.Touches),
		time.Duration(req.TimeMilliSeconds)*time.Millisecond) {
		return nil, errors.Wrap(err, "failed to swipe")
	}

	if err := tw.End(); err != nil {
		return nil, errors.Wrap(err, "failed to finish the swipe")
	}

	return &empty.Empty{}, nil
}

// getDisplayDimensions retrieves the dimension for the main display.
func (svc *TouchService) getDisplayDimensions(ctx context.Context) (int, int, error) {
	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	cr := svc.sharedObject.Chrome
	if cr == nil {
		return -1, -1, errors.New("Chrome is not instantiated")
	}

	tconn, err := cr.TestAPIConn(ctx)
	info, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		return -1, -1, errors.Wrap(err, "failed to get display")
	}
	return info.Bounds.Width, info.Bounds.Height, nil
}
