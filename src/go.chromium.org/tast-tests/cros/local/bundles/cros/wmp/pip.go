// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	arcPipTestPkgName   = "org.chromium.arc.testapp.pictureinpicture"
	arcPipAppName       = "ArcPipTest"
	tuckHandleClassName = "TuckHandleWidget"
)

type pipTestFunc func(context.Context, *chrome.TestConn, *uiauto.Context, *display.Info, *input.TouchscreenEventWriter) error
type pipType int

const (
	ashPip pipType = iota
	lacrosPip
	arcPip
)

type pipTestParams struct {
	pipType       pipType
	tests         []pipTestFunc
	browserType   browser.Type
	pipWindowName string
	pipClassName  string
}

var ashPipTests = pipTestParams{
	pipType: ashPip,
	tests: []pipTestFunc{
		testPipPinchResize,
		testPipDoubleTapToResize,
		testPipTuck,
		testPipMove,
		testPipExpandViaMenu,
	},
	browserType:   browser.TypeAsh,
	pipWindowName: "Picture in picture",
	pipClassName:  "PictureInPictureWindow",
}

var lacrosPipTests = pipTestParams{
	pipType: lacrosPip,
	tests: []pipTestFunc{
		testPipPinchResize,
		testPipDoubleTapToResize,
		testPipTuck,
		testPipMove,
		testPipExpandViaMenu,
	},
	browserType:   browser.TypeLacros,
	pipWindowName: "Picture in picture",
	pipClassName:  "Widget",
}

var arcPipTests = pipTestParams{
	pipType: arcPip,
	tests: []pipTestFunc{
		testPipPinchResize,
		testPipDoubleTapToResize,
		testPipTuck,
		testPipMove,
		testPipExpandViaMenu,
		testPipExpandViaShelfIcon,
	},
	browserType:   browser.TypeAsh,
	pipWindowName: arcPipAppName,
	pipClassName:  "Widget",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Pip,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests window behavior of PiP windows",
		Contacts: []string{
			"chromeos-wm-corexp@google.com",
			"chromeos-sw-engprod@google.com",
			"takise@chromium.org",
		},
		// ChromeOS > Software > Window Management > PiP Window
		BugComponent: "b:1252568",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Data:         []string{"180p_60fps_600frames.h264.mp4", "pip_video.html"},
		SoftwareDeps: []string{"chrome", "arc", "proprietary_codecs"},
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 2*time.Minute,
		Params: []testing.Param{{
			Val: ashPipTests,
		}, {
			Name:              "lacros",
			Val:               lacrosPipTests,
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:              "arc",
			Val:               arcPipTests,
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func Pip(ctx context.Context, s *testing.State) {
	testParams := s.Param().(pipTestParams)

	// Enable feature flag for PiP Double-Tap-to-Resize feature and PiP Tuck feature.
	opts := []chrome.Option{chrome.EnableFeatures("PipDoubleTapToResize"),
		chrome.EnableFeatures("PipTuck"), chrome.ExtraArgs("--show-taps")}

	switch testParams.pipType {
	case arcPip:
		opts = append(opts, chrome.ARCEnabled())
	case lacrosPip:
		lacrosOpts, err := lacrosfixt.NewConfig().Opts()
		if err != nil {
			s.Fatal("Failed to get default Lacros options: ", err)
		}
		opts = append(opts, lacrosOpts...)
	}

	cr, err := chrome.New(ctx, opts...)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get primary display info: ", err)
	}

	orientation, err := display.GetOrientation(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the display orientation: ", err)
	}

	cleanupShelfAlignment, err := ash.EnsureShelfAlignmentBottom(ctx, tconn, dispInfo.ID)
	if err != nil {
		s.Fatal("Failed to ensure shelf alignment is Bottom: ", err)
	}
	defer cleanupShelfAlignment(cleanupCtx)

	cleanupShelfBehavior, err := ash.EnsureShelfBehavior(ctx, tconn, dispInfo.ID, ash.ShelfBehaviorNeverAutoHide)
	if err != nil {
		s.Fatal("Failed to ensure shelf behavior is Never Auto Hide: ", err)
	}
	defer cleanupShelfBehavior(cleanupCtx)

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	ac := uiauto.New(tconn)

	var a *arc.ARC
	var pipAct *arc.Activity
	if testParams.pipType == arcPip {
		a, err = arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to launch ARC: ", err)
		}

		const apkName = "ArcPipTest.apk"
		if err := a.Install(ctx, arc.APKPath(apkName)); err != nil {
			s.Fatal("Failed installing PiP app: ", err)
		}
	}

	// Run each of the tests.
	for _, fn := range testParams.tests {
		func() {
			// Show a new PiP window.
			if testParams.pipType == arcPip {
				pipAct, err = createArcPip(ctx, cr, tconn, a, dispInfo, testParams)
				if err != nil {
					s.Fatal("Failed to create an ARC PiP window: ", err)
				}
				defer cleanUpArcTest(cleanupCtx, tconn, pipAct)
			} else {
				closeBrowser, err := createBrowserPip(ctx, cr, tconn, ac, s.DataFileSystem(), testParams)
				if err != nil {
					s.Fatal("Failed to create a browser PiP window: ", err)
				}
				defer closeBrowser(cleanupCtx)
			}

			// Get touch event writers.
			tsw, err := input.Touchscreen(ctx)
			if err != nil {
				s.Fatal("Failed to get touchscreen event writer: ", err)
			}
			defer tsw.Close(ctx)

			if err := tsw.SetRotation(-orientation.Angle); err != nil {
				s.Fatal("Failed to set touchscreen event writer rotation: ", err)
			}

			// Execute the tests.
			if err := fn(ctx, tconn, ac, dispInfo, tsw); err != nil {
				s.Fatal("Failed to execute test: ", err)
			}
		}()
	}
}

func testPipPinchResize(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, dispInfo *display.Info, tsw *input.TouchscreenEventWriter) error {
	mtw, err := tsw.NewMultiTouchWriter(2)
	if err != nil {
		return errors.Wrap(err, "failed to get touch event writer")
	}
	defer mtw.Close()

	window, err := getPIPWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get PiP window")
	}
	beforeBounds := window.BoundsInRoot

	// Perform pinch gesture on the PiP window.
	tcc := tsw.NewTouchCoordConverter(dispInfo.Bounds.Size())
	offset := coords.NewPoint(beforeBounds.Width/4, beforeBounds.Height/4)

	rightStartX, rightStartY := tcc.ConvertLocation(beforeBounds.CenterPoint().Add(offset))
	rightEndX, rightEndY := tcc.ConvertLocation(beforeBounds.CenterPoint().Add(offset).Add(offset))

	leftStartX, leftStartY := tcc.ConvertLocation(beforeBounds.CenterPoint().Sub(offset))
	leftEndX, leftEndY := tcc.ConvertLocation(beforeBounds.CenterPoint().Sub(offset).Sub(offset))

	// Here, `Zoom()` is not used. With `Zoom()`, the zoom-in pinch
	// gesture begins with the two fingers in the exact same location,
	// and therefore the gesture is not correctly registered in the
	// client side.
	if err := mtw.Pinch(ctx, leftStartX, leftStartY, leftEndX, leftEndY,
		rightStartX, rightStartY, rightEndX, rightEndY, time.Second); err != nil {
		return errors.Wrap(err, "failed to perform pinch zoom")
	}

	// Confirm that the window has resized due to the gesture.
	window, err = getPIPWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get PiP window")
	}
	afterBounds := window.BoundsInRoot
	if beforeBounds.Width >= afterBounds.Width ||
		beforeBounds.Height >= afterBounds.Height {
		return errors.Wrapf(err, "unexpected PiP window size; window size changed from %v to %v",
			beforeBounds, afterBounds)
	}

	return nil
}

// testPipDoubleTapToResize verifies that PiP gets enlarged as expected via the double-tap-to-resize feature.
// TODO(b/314875724): Add more test cases for this feature.
func testPipDoubleTapToResize(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, dispInfo *display.Info, tsw *input.TouchscreenEventWriter) error {
	window, err := getPIPWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get PiP window")
	}
	beforeBounds := window.BoundsInRoot
	// Target the upper part of the PiP window so we don't click on any button accidentally.
	clickPoint := beforeBounds.CenterPoint().Sub(coords.NewPoint(0, beforeBounds.Height/4))
	if err := mouse.DoubleClick(tconn, clickPoint, 100*time.Millisecond)(ctx); err != nil {
		return errors.Wrap(err, "failed to double-click PiP")
	}

	// Confirm that the window has been enlarged via the double-tap-to-resize feature.
	return testing.Poll(ctx, func(ctx context.Context) error {
		window, err = getPIPWindow(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get PiP window")
		}
		afterBounds := window.BoundsInRoot
		if beforeBounds.Width >= afterBounds.Width {
			return errors.Wrapf(err, "unexpected PiP window width; want: width>%v, actual: width=%v", beforeBounds.Width, afterBounds.Width)
		}
		if beforeBounds.Height >= afterBounds.Height {
			return errors.Wrapf(err, "unexpected PiP window height; want: height>%v, actual: height=%v", beforeBounds.Height, afterBounds.Height)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

func testPipTuck(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, dispInfo *display.Info, tsw *input.TouchscreenEventWriter) error {
	tcc := tsw.NewTouchCoordConverter(dispInfo.Bounds.Size())

	stw, err := tsw.NewSingleTouchWriter()
	if err != nil {
		return errors.Wrap(err, "failed to get touch event writer")
	}
	defer stw.Close()

	// Attempt to tuck the window to the right.
	window, err := getPIPWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get PiP window")
	}
	// Lacros PiP can't be drag resized by grabbing one of the buttons on the menu.
	// Grab the left part of PiP, where no UI can be placed on PiP menu.
	startPoint := window.BoundsInRoot.CenterPoint().Sub(coords.NewPoint(window.BoundsInRoot.Width/4, 0))
	beforeX, beforeY := tcc.ConvertLocation(startPoint)
	// Inputting an event to the exact right edge is considered invalid.
	afterX, afterY := tcc.ConvertLocation(coords.NewPoint(dispInfo.Bounds.Width-1, startPoint.Y))
	if err := stw.Swipe(ctx, beforeX, beforeY, afterX, afterY, time.Second); err != nil {
		return errors.Wrap(err, "failed to swipe the window to edge")
	}
	stw.End()

	// Confirm that the window has been tucked by checking if tuck handle is shown first.
	tuckHandle := nodewith.HasClass(tuckHandleClassName)
	if err := ac.WaitUntilExists(tuckHandle)(ctx); err != nil {
		return errors.Wrap(err, "failed to locate tuck handle")
	}

	// Check other conditions that need to be held while PiP is tucked.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		window, err := getPIPWindow(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to Get PIP window"))
		}
		if window.IsAnimating {
			return errors.New("PiP is still animating")
		}
		// Don't be too exact and verify less than on fourth of the area of PiP is visible.
		tuckPositionThreshold := dispInfo.Bounds.Width - window.BoundsInRoot.Width/4
		if window.BoundsInRoot.Left <= tuckPositionThreshold {
			return errors.Wrapf(err, "unexpected PiP window position; want: left position>%v, actual: left position=%v",
				tuckPositionThreshold, window.BoundsInRoot.Left)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to tuck PiP")
	}

	// Untuck PiP and confirm it by checking if tuck handle is gone first.
	if err := ac.WithTimeout(10 * time.Second).LeftClick(tuckHandle)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on tuck handle")
	}
	if err := ac.WaitUntilGone(tuckHandle)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for tuck handle to be gone")
	}

	// Check other conditions that need to be held while PiP is not tucked.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		window, err := getPIPWindow(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to Get PIP window"))
		}
		if window.IsAnimating {
			return errors.New("PiP is still animating")
		}
		if window.BoundsInRoot.Right() >= dispInfo.Bounds.Width {
			return errors.Wrapf(err, "unexpected PiP window position; want: right position<%v, actual: right position=%v",
				dispInfo.Bounds.Width, window.BoundsInRoot.Right())
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to untuck PiP")
	}

	return nil
}

// testPipMove verifies that drag-moving the PIP window works as expected.
func testPipMove(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, dispInfo *display.Info, tsw *input.TouchscreenEventWriter) error {
	stw, err := tsw.NewSingleTouchWriter()
	if err != nil {
		return errors.Wrap(err, "failed to get touch event writer")
	}
	defer stw.Close()

	window, err := getPIPWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get PiP window")
	}
	beforeBounds := window.BoundsInRoot

	// Perform swipe gesture on the PiP window.
	tcc := tsw.NewTouchCoordConverter(dispInfo.Bounds.Size())
	offset := coords.NewPoint(dispInfo.Bounds.Width/2, 0)

	// Lacros PiP can't be drag resized by grabbing one of the buttons on the menu.
	// Grab the upper part of PiP, where no UI can be placed on PiP menu.
	start := coords.Point{X: beforeBounds.CenterX(), Y: beforeBounds.Top + beforeBounds.Height/5}
	startX, startY := tcc.ConvertLocation(start)
	endX, endY := tcc.ConvertLocation(start.Sub(offset))

	if err := stw.Swipe(ctx, startX, startY, endX, endY, time.Second); err != nil {
		return errors.Wrap(err, "failed to perform swipe move")
	}
	if err := stw.End(); err != nil {
		return errors.Wrap(err, "failed to finish swipe gesture")
	}

	// Confirm that the window has moved due to the gesture.
	// Note that PiP drag move is asynchronously started by the client, we can't assert the exact position.
	return testing.Poll(ctx, func(ctx context.Context) error {
		window, err = getPIPWindow(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get PiP window")
		}
		afterBounds := window.BoundsInRoot
		if beforeBounds.Left <= afterBounds.Left {
			return errors.Wrapf(err, "unexpected PiP window position; want: left position<%v, actual: left position=%v", beforeBounds.Left, afterBounds.Left)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// testPipExpandViaMenu verifies that PiP can be expanded by tapping the center of the menu overlay of the window.
func testPipExpandViaMenu(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, dispInfo *display.Info, tsw *input.TouchscreenEventWriter) error {
	stw, err := tsw.NewSingleTouchWriter()
	if err != nil {
		return errors.Wrap(err, "failed to get touch event writer")
	}
	defer stw.Close()

	window, err := getPIPWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get PiP window")
	}
	bounds := window.BoundsInRoot

	tcc := tsw.NewTouchCoordConverter(dispInfo.Bounds.Size())
	centerX, centerY := tcc.ConvertLocation(bounds.CenterPoint())

	// Tap the center of the PiP menu, where the expand button exists for any type of PiP.
	// Here, we retry tapping until PiP is gone because we don't know when PiP menu starts accepting input events (PiP menu is rendered on the client side).
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := stw.Move(centerX, centerY); err != nil {
			return errors.Wrap(err, "failed to tap the center of PiP")
		}
		if err := stw.End(); err != nil {
			return errors.Wrap(err, "failed to finish tap gesture")
		}
		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get the window list"))
		}
		for _, window := range ws {
			if window.IsVisible && window.State == ash.WindowStatePIP {
				return errors.New("PiP still exists")
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// testPipExpandViaShelfIcon verifies that PiP can be expanded by pressing the shelf icon of the app.
func testPipExpandViaShelfIcon(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, _ *display.Info, _ *input.TouchscreenEventWriter) error {
	return expandPipViaShelfIcon(ctx, tconn, ac)
}

// expandPipViaShelfIcon expands the PiP window by pressing the shelf icon of the app.
// Note that this behavior is currently supported only by ARC PiP.
func expandPipViaShelfIcon(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context) error {
	pipShelfIcon := nodewith.Name(arcPipAppName).ClassName(ash.ShelfAppButtonClassName)
	if err := ac.WithTimeout(10 * time.Second).LeftClick(pipShelfIcon)(ctx); err != nil {
		return errors.Wrapf(err, "failed to click on the shelf icon of %s", arcPipAppName)
	}
	return waitUntilPipWindowIsGone(ctx, tconn)
}

func createArcPip(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, a *arc.ARC, dispInfo *display.Info, test pipTestParams) (*arc.Activity, error) {
	pipAct, err := arc.NewActivity(a, arcPipTestPkgName, ".PipActivity")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create PiP activity")
	}

	if err := pipAct.Start(ctx, tconn); err != nil {
		return pipAct, errors.Wrapf(err, "failed to start %s", pipAct.ActivityName())
	}

	if err := ash.WaitForVisible(ctx, tconn, pipAct.PackageName()); err != nil {
		return pipAct, errors.Wrap(err, "failed to wait for PiP activity to be visible")
	}

	window, err := ash.GetARCAppWindowInfo(ctx, tconn, pipAct.PackageName())
	if err != nil {
		return pipAct, errors.Wrapf(err, "failed to get ARC window infomation for package name %s", pipAct.ActivityName())
	}

	// The window is minimized here, but the expected state is PiP, so the async API must used.
	if _, err := ash.SetWindowState(ctx, tconn, window.ID, ash.WMEventMinimize, false /* waitForStateChange */); err != nil {
		return pipAct, errors.Wrapf(err, "failed to minimize %s", pipAct.ActivityName())
	}

	if err := waitForPipWindow(ctx, tconn); err != nil {
		return pipAct, errors.Wrap(err, "failed to enter PiP mode")
	}

	return pipAct, nil
}

func createBrowserPip(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, ac *uiauto.Context, dataFS http.FileSystem, test pipTestParams) (func(ctx context.Context) error, error) {
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, test.browserType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the browser")
	}

	kw, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get keyboard event writer")
	}
	defer kw.Close(ctx)

	pc := pointer.NewMouse(tconn)
	defer pc.Close(ctx)

	srv := httptest.NewServer(http.FileServer(dataFS))
	defer srv.Close()

	conn, err := br.NewConn(ctx, srv.URL+"/pip_video.html")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load pip_video.html")
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to wait for pip_video.html to quiesce")
	}

	// Show the PiP window.
	pipButton := nodewith.Name("PIP").Role(role.Button)
	if err := ac.WithTimeout(10 * time.Second).LeftClick(pipButton)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to click on the PiP button")
	}

	if err := waitForPipWindow(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to enter PiP mode")
	}

	return func(ctx context.Context) error {
		if err := closeBrowser(ctx); err != nil {
			return errors.Wrap(err, "failed to close browser window")
		}
		if err := ash.CloseAllWindows(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all windows")
		}
		return nil
	}, nil
}

// waitForPipWindow keeps looking for a PiP window until it appears on the Chrome side.
func waitForPipWindow(ctx context.Context, tconn *chrome.TestConn) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		w, err := getPIPWindow(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "the PiP window hasn't been created yet")
		}
		if !w.IsVisible || w.IsAnimating {
			return errors.New("the PiP window is not in a stable state yet")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// getPIPWindow returns the PIP window if any.
func getPIPWindow(ctx context.Context, tconn *chrome.TestConn) (*ash.Window, error) {
	return ash.FindWindow(ctx, tconn, func(w *ash.Window) bool { return w.State == ash.WindowStatePIP })
}

// cleanUpArcTest cleans up all the windows that get shown at the set-up of an test for ARC.
func cleanUpArcTest(ctx context.Context, tconn *chrome.TestConn, pipAct *arc.Activity) error {
	if err := pipAct.Stop(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to stop ARC PiP activity")
	}
	pipAct.Close(ctx)
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to close all windows")
	}
	return nil
}

// waitUntilPipWindowIsGone keeps looking for a PiP window until it gets gone.
func waitUntilPipWindowIsGone(ctx context.Context, tconn *chrome.TestConn) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get the window list"))
		}
		for _, window := range ws {
			if window.State == ash.WindowStatePIP {
				return errors.New("PiP still exists")
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}
