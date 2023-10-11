// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"image"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var displayService DisplayService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			displayService = DisplayService{sharedObject: common.SharedObjectsForServiceSingleton}
			wwcb.RegisterDisplayServiceServer(srv, &displayService)
		},
		GuaranteeCompatibility: true,
	})
}

// Timeout & interval for verifying display when a dock interacts with Chromebook.
const (
	displayTimeout  = 30 * time.Second
	displayInterval = 200 * time.Millisecond
)

// DisplayService implements tast.cros.wwcb.DisplayService.
type DisplayService struct {
	sharedObject *common.SharedObjectsForService
}

// SetMirrorDisplay enables or disables mirror mode in display settings.
func (ds *DisplayService) SetMirrorDisplay(ctx context.Context, req *wwcb.QueryRequest) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Test API connection")
	}

	// Set mirror display.
	var want checked.Checked
	if bool(req.Enable) {
		want = checked.True
	} else {
		want = checked.False
	}
	if err := utils.SetMirrorDisplay(ctx, tconn, want); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to set mirror display")
	}

	// Expect the display is changed. Return err after poll timeout.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		intDispInfo, err := display.GetInternalInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get display infos in mirror mode")
		}

		if bool(req.Enable) {
			if intDispInfo.ID != intDispInfo.MirroringSourceID {
				return errors.Errorf("unexcepted mirror source ID: got %s, want %s", intDispInfo.MirroringSourceID, intDispInfo.ID)
			}
		} else {
			if intDispInfo.ID == intDispInfo.MirroringSourceID {
				return errors.New("mirror source ID should be empty string")
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return &empty.Empty{}, err
	}

	return &empty.Empty{}, nil
}

// SetPrimaryDisplay sets the internal or external display as primary mode.
func (ds *DisplayService) SetPrimaryDisplay(ctx context.Context, req *wwcb.QueryRequest) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Test API connection")
	}

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return &empty.Empty{}, err
	}

	if int(req.DisplayIndex) >= len(infos) {
		return &empty.Empty{}, errors.New("display index is out of range")
	}

	// Set the display to primary.
	displayID := infos[int(req.DisplayIndex)].ID
	isPrimary := true
	if err := display.SetDisplayProperties(ctx, tconn, displayID, display.DisplayProperties{IsPrimary: &isPrimary}); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to set display properties")
	}

	// Expect the display is changed. Return err after poll timeout.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		primaryInfo, err := display.GetPrimaryInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get primary display info")
		}
		if primaryInfo.ID != displayID {
			return errors.New("unable to set display as primary")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return &empty.Empty{}, err
	}

	return &empty.Empty{}, nil
}

// SwitchWindowToDisplay finds the window with given title then switch it to the expected display.
func (ds *DisplayService) SwitchWindowToDisplay(ctx context.Context, req *wwcb.QueryRequest) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, err
	}
	defer kb.Close(ctx)

	w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
		return w.Title == string(req.WindowTitle)
	})
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to find window")
	}

	if err := w.ActivateWindow(ctx, tconn); err != nil {
		return &empty.Empty{}, err
	}

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get display info")
	}

	if int(req.DisplayIndex) >= len(infos) {
		return &empty.Empty{}, errors.New("display index is out of range")
	}

	wantDispID := infos[int(req.DisplayIndex)].ID
	if w.DisplayID != wantDispID {
		testing.ContextLog(ctx, "Expected window not found")
		testing.ContextLogf(ctx, "Switch window %q to %s", w.Title, wantDispID)

		if err := kb.AccelAction("Search+Alt+M")(ctx); err != nil {
			return &empty.Empty{}, errors.Wrap(err, "failed to type Search+Alt+M")
		}

		w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return w.Title == string(req.WindowTitle)
		})
		if err != nil {
			return &empty.Empty{}, errors.Wrap(err, "failed to find window")
		}
		if w.DisplayID != wantDispID {
			return &empty.Empty{}, errors.Errorf("failed to switch window to %s", wantDispID)
		}
	}
	return &empty.Empty{}, nil
}

// VerifyWindowOnDisplay finds the window with given title then verify it is showing on the expected display or not.
func (ds *DisplayService) VerifyWindowOnDisplay(ctx context.Context, req *wwcb.QueryRequest) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return &empty.Empty{}, err
	}

	// Retries to check window & display info, cause system might need time to apply the changes.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return w.Title == string(req.WindowTitle)
		})
		if err != nil {
			return errors.Wrap(err, "failed to find window")
		}

		infos, err := display.GetInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get display info")
		}

		if int(req.DisplayIndex) >= len(infos) {
			return errors.New("display index is out of range")
		}

		displayID := infos[int(req.DisplayIndex)].ID
		if w.DisplayID != displayID && w.IsVisible && w.IsFrameVisible {
			return errors.Errorf("window isn't showing on certain display: got %s, want %s", w.DisplayID, displayID)
		}

		return nil
	}, &testing.PollOptions{Timeout: displayTimeout, Interval: displayInterval}); err != nil {
		return &empty.Empty{}, err
	}

	return &empty.Empty{}, nil
}

// VerifyDisplayCount verifies the given  display count to compare with the current numbers of display that system detected.
func (ds *DisplayService) VerifyDisplayCount(ctx context.Context, req *wwcb.QueryRequest) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return &empty.Empty{}, err
	}

	// Retries to check window & display info, cause system might need time to apply the changes.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		infos, err := display.GetInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get display info")
		}

		if len(infos) != int(req.DisplayCount) {
			return errors.Errorf("unexpected number of displays: got %d, want %d", len(infos), int(req.DisplayCount))
		}
		return nil
	}, &testing.PollOptions{Timeout: displayTimeout, Interval: displayInterval}); err != nil {
		return &empty.Empty{}, err
	}

	return &empty.Empty{}, nil
}

// ChangeResolution changes the given display's resolution, respectively low, medium and high resolution.
func (ds *DisplayService) ChangeResolution(ctx context.Context, req *wwcb.QueryRequest) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get external display info")
	}

	if int(req.DisplayIndex) >= len(infos) {
		return nil, errors.New("display index is out of range")
	}

	// Set the given display's low, medium and high resolution.
	info := infos[req.DisplayIndex]
	if len(info.Modes) < 3 {
		return nil, errors.New("display modes are not enough")
	}

	low := info.Modes[0]
	medium := info.Modes[(len(info.Modes)-1)/2]
	high := info.Modes[len(info.Modes)-1]

	for _, param := range []struct {
		displayMode display.DisplayMode
	}{
		{*low}, {*medium}, {*high},
	} {
		setWidth := param.displayMode.Width
		setHeight := param.displayMode.Height
		testing.ContextLogf(ctx, "Setting resolution: %d x %d", setWidth, setHeight)

		p := display.DisplayProperties{DisplayMode: &param.displayMode}
		if err := display.SetDisplayProperties(ctx, tconn, info.ID, p); err != nil {
			return nil, errors.Wrap(err, "failed to set display properties")
		}

		// Poll is required as completion of display.SetDisplayProperties.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			infos, err := display.GetInfo(ctx, tconn)
			if err != nil {
				return errors.Wrap(err, "failed to get display info")
			}

			if int(req.DisplayIndex) >= len(infos) {
				return errors.New("display index is out of range")
			}

			changedInfo := infos[req.DisplayIndex]
			if changedInfo.Bounds.Width != setWidth || changedInfo.Bounds.Height != setHeight {
				return errors.New("the display mode has not changed yet")
			}
			return nil
		}, &testing.PollOptions{Timeout: displayTimeout, Interval: displayInterval}); err != nil {
			return nil, errors.Wrap(err, "failed to verfiy display properties")
		}

		// Compare display mode with screenshot of external display.
		fd, err := ioutil.TempFile("", "screenshot")
		if err != nil {
			return nil, errors.Wrap(err, "error opening screenshot file")
		}
		defer os.Remove(fd.Name())
		defer fd.Close()

		if err := screenshot.CaptureChromeForDisplay(ctx, cr, info.ID, fd.Name()); err != nil {
			return nil, errors.Wrap(err, "failed to capture screenshot")
		}

		img, _, err := image.Decode(fd)
		if err != nil {
			return nil, errors.Wrap(err, "error decoding image")
		}
		imgWidth := img.Bounds().Dx()
		imgHeight := img.Bounds().Dy()

		if imgWidth != setWidth || imgHeight != setHeight {
			return nil, errors.Errorf("unexpected resolution, got: %dx%d, want: %dx%d", imgWidth, imgHeight, setWidth, setHeight)
		}
	}

	return &empty.Empty{}, nil
}

// ChangeRelativePosition changes position of external display relative to Chromebook and check window is still on external display.
func (ds *DisplayService) ChangeRelativePosition(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a keyboard")
	}
	defer kb.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Open any window to for testing requirement.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch filesapp")
	}
	defer files.Close(cleanupCtx)

	if err := utils.SwitchWindowToDisplay(ctx, tconn, kb, true)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to switch window to external display")
	}

	infos, err := utils.GetInternalAndExternalDisplays(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get internal and external display info")
	}
	extDispInfo := infos.External
	intDispInfo := infos.Internal

	// Relayout external display and make sure the windows will not move their positions or show black background.
	for _, relayout := range []struct {
		name   string
		offset coords.Point
	}{
		{"Relayout external display on top of internal display", coords.NewPoint(0, -extDispInfo.Bounds.Height)},
		{"Relayout external display on bottom of internal display", coords.NewPoint(0, intDispInfo.Bounds.Height)},
		{"Relayout external display to the left side of internal display", coords.NewPoint(-extDispInfo.Bounds.Width, 0)},
		{"Relayout external display to the right side of internal display", coords.NewPoint(intDispInfo.Bounds.Width, 0)},
	} {
		p := display.DisplayProperties{BoundsOriginX: &relayout.offset.X, BoundsOriginY: &relayout.offset.Y}
		if err := display.SetDisplayProperties(ctx, tconn, extDispInfo.ID, p); err != nil {
			return nil, errors.Wrap(err, "failed to set display properties")
		}

		// Poll is required as completion of display.SetDisplayProperties.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			extDispInfo, err := display.FindInfo(ctx, tconn, func(info *display.Info) bool { return info.IsInternal == false })
			if err != nil {
				return errors.Wrap(err, "failed to find external display")
			}
			if extDispInfo.Bounds.Left != relayout.offset.X || extDispInfo.Bounds.Top != relayout.offset.Y {
				return errors.New("display origin has not been updated")
			}
			return nil
		}, &testing.PollOptions{Timeout: displayTimeout, Interval: displayInterval}); err != nil {
			return nil, err
		}

		w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return strings.HasPrefix(w.Title, filesapp.FilesTitlePrefix)
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to find filesapp window")
		}
		if w.DisplayID != extDispInfo.ID {
			return nil, errors.Errorf("window shows on wrong display, got: %s, want: %s", w.DisplayID, extDispInfo.ID)
		}
	}

	return &empty.Empty{}, nil
}

// VerifyAfterLidClose verifies that display resolution is still okay after lid close & windows are all still displayed.
func (ds *DisplayService) VerifyAfterLidClose(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Open any window to for testing requirement.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch filesapp")
	}
	defer files.Close(cleanupCtx)

	before, err := display.FindInfo(ctx, tconn, func(info *display.Info) bool { return info.IsInternal == false })
	if err != nil {
		return nil, errors.Wrap(err, "failed to find external display info")
	}

	// Close display power as an alternative way to close lid.
	if err := power.SetDisplayPower(ctx, power.DisplayPowerInternalOffExternalOn); err != nil {
		return nil, errors.Wrap(err, "failed to set display power")
	}

	// Poll is required as display response and window jump.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Compare external display resolution before and after lid close.
		after, err := display.FindInfo(ctx, tconn, func(info *display.Info) bool { return info.IsInternal == false })
		if err != nil {
			return errors.Wrap(err, "failed to find external display info after lid close")
		}

		if before.Bounds.Width != after.Bounds.Width || before.Bounds.Height != after.Bounds.Height {
			return errors.Errorf("resolution is different, original: %dx%d, after close lid: %dx%d", before.Bounds.Width, before.Bounds.Height, after.Bounds.Width, before.Bounds.Height)
		}

		// Check window bounds on external display.
		w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return strings.HasPrefix(w.Title, filesapp.FilesTitlePrefix)
		})
		if err != nil {
			return errors.Wrap(err, "failed to find filesapp window")
		}

		if w.DisplayID != after.ID {
			return errors.Errorf("window shows on wrong display, got: %s, want: %s", w.DisplayID, after.ID)
		}
		return nil
	}, &testing.PollOptions{Timeout: displayTimeout, Interval: displayInterval}); err != nil {
		return nil, errors.Wrap(err, "failed to verify window and display resolution")
	}

	if err := power.SetDisplayPower(cleanupCtx, power.DisplayPowerAllOn); err != nil {
		return nil, errors.Wrap(err, "failed to turn on all display power")
	}

	// Expect the app moves back to the internal display when powering on again.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return strings.HasPrefix(w.Title, filesapp.FilesTitlePrefix)
		})
		if err != nil {
			return errors.Wrap(err, "failed to find filesapp window")
		}

		if w.DisplayID != before.ID {
			return errors.Errorf("window shows on wrong display, got: %s, want: %s", w.DisplayID, before.ID)
		}
		return nil
	}, &testing.PollOptions{Timeout: displayTimeout, Interval: displayInterval}); err != nil {
		return nil, errors.Wrap(err, "failed to verify window show on internal display")
	}

	return &empty.Empty{}, nil
}

// GetDisplayIDs returns the display ID of all monitor from system information.
func (ds *DisplayService) GetDisplayIDs(ctx context.Context, req *empty.Empty) (*wwcb.GetDisplayIDsResponse, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get external display info")
	}

	var displayIDArray []string
	for _, info := range infos {
		displayIDArray = append(displayIDArray, info.ID)
	}

	var resp = &wwcb.GetDisplayIDsResponse{
		DisplayIds: displayIDArray,
	}

	return resp, nil
}

// EnsureSetWindowState checks whether the window is in requested window state. If not, make sure to set window state to the requested window state.
func (ds *DisplayService) EnsureSetWindowState(ctx context.Context, req *wwcb.QueryRequest) (*empty.Empty, error) {
	cr := ds.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome is not instantiated")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
		return w.Title == string(req.WindowTitle)
	})
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to find window")
	}

	windowTypeMap := map[wwcb.WindowStateType]ash.WindowStateType{
		wwcb.WindowStateType_WINDOW_STATE_NORMAL:     ash.WindowStateNormal,
		wwcb.WindowStateType_WINDOW_STATE_MAXIMIZED:  ash.WindowStateMaximized,
		wwcb.WindowStateType_WINDOW_STATE_MINIMIZED:  ash.WindowStateMinimized,
		wwcb.WindowStateType_WINDOW_STATE_FULLSCREEN: ash.WindowStateFullscreen,
	}
	wmType, ok := windowTypeMap[req.WindowState]
	if !ok {
		return nil, errors.Errorf("didn't find the type for window state %q", req.WindowState)
	}

	if wmType == w.State {
		return &empty.Empty{}, nil
	}

	windowEventMap := map[wwcb.WindowStateType]ash.WMEventType{
		wwcb.WindowStateType_WINDOW_STATE_NORMAL:     ash.WMEventNormal,
		wwcb.WindowStateType_WINDOW_STATE_MAXIMIZED:  ash.WMEventMaximize,
		wwcb.WindowStateType_WINDOW_STATE_MINIMIZED:  ash.WMEventMinimize,
		wwcb.WindowStateType_WINDOW_STATE_FULLSCREEN: ash.WMEventFullscreen,
	}
	wmEvent, ok := windowEventMap[req.WindowState]
	if !ok {
		return nil, errors.Errorf("didn't find the event for window state %q", req.WindowState)
	}

	state, err := ash.SetARCAppWindowState(ctx, tconn, w.ARCPackageName, wmEvent)
	if err != nil {
		return nil, err
	}

	if state != wmType {
		return nil, errors.Errorf("unexpected window state; got %s, want %s", state, wmType)
	}
	if err := ash.WaitForARCAppWindowState(ctx, tconn, w.ARCPackageName, wmType); err != nil {
		return nil, errors.Wrapf(err, "failed to wait for activity to enter %v state", wmType)
	}

	return &empty.Empty{}, nil
}
