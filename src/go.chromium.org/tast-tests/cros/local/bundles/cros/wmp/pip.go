// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing/hwdep"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	arcPipTestPkgName = "org.chromium.arc.testapp.pictureinpicture"
)

type pipTestFunc func(context.Context, *uiauto.Context, *display.Info, *nodewith.Finder) error

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
	},
	browserType:   browser.TypeAsh,
	pipWindowName: "Picture in picture",
	pipClassName:  "PictureInPictureWindow",
}

var lacrosPipTests = pipTestParams{
	pipType: lacrosPip,
	tests: []pipTestFunc{
		testPipPinchResize,
	},
	browserType:   browser.TypeLacros,
	pipWindowName: "Picture in picture",
	pipClassName:  "Widget",
}

var arcPipTests = pipTestParams{
	pipType: arcPip,
	tests: []pipTestFunc{
		testPipPinchResize,
	},
	browserType:   browser.TypeAsh,
	pipWindowName: "ArcPipTest",
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
			"massan@google.com",
		},
		// ChromeOS > Software > Window Management > PiP Window
		BugComponent: "b:1252568",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Data:         []string{"180p_60fps_600frames.h264.mp4", "pip_video.html"},
		HardwareDeps: hwdep.D(hwdep.TouchScreen()),
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

	// Enable feature flag for PiP Pinch-to-Resize feature.
	opts := []chrome.Option{chrome.EnableFeatures("PipPinchToResize"),
		chrome.ExtraArgs("--show-taps")}

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

	// Run each of the tests.
	for _, fn := range testParams.tests {
		// Show a new PiP window.
		if testParams.pipType == arcPip {
			if err := createArcPip(ctx, cr, tconn, dispInfo, s.OutDir(), testParams); err != nil {
				s.Fatal("Failed to create an ARC PiP window: ", err)
			}
		} else {
			closeBrowser, err := createBrowserPip(ctx, cr, tconn, ac, s.DataFileSystem(), testParams)
			if err != nil {
				s.Fatal("Failed to create a browser PiP window: ", err)
			}
			defer closeBrowser(cleanupCtx)
		}

		// Get the PiP window. Browser PiP window cannot be able to be
		// retrieved with `ash.FindWindow()`, as it's not included in
		// `mruWindowTracker::BuildAppWindowList`.
		pipWindow :=
			nodewith.Name(testParams.pipWindowName).ClassName(testParams.pipClassName).Onscreen().First()

		// Execute the tests.
		if err := fn(ctx, ac, dispInfo, pipWindow); err != nil {
			s.Fatal("Failed to execute test: ", err)
		}
	}
}

func testPipPinchResize(ctx context.Context, ac *uiauto.Context, dispInfo *display.Info, pipWindow *nodewith.Finder) error {
	// Get touch event writers.
	tsw, err := input.Touchscreen(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get touchscreen event writer")
	}
	defer tsw.Close(ctx)

	mtw, err := tsw.NewMultiTouchWriter(2)
	if err != nil {
		return errors.Wrap(err, "failed to get touch event writer")
	}
	defer mtw.Close()

	beforeBounds, err := ac.Location(ctx, pipWindow)
	if err != nil {
		return errors.Wrap(err, "failed to get bounds")
	}

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
	afterBounds, err := ac.Location(ctx, pipWindow)
	if err != nil {
		return errors.Wrap(err, "failed to get bounds for the PiP window")
	}
	if beforeBounds.Width >= afterBounds.Width ||
		beforeBounds.Height >= afterBounds.Height {
		return errors.Wrapf(err, "unexpected PiP window size; window size changed from %v to %v",
			beforeBounds, afterBounds)
	}

	return nil
}

func createArcPip(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, dispInfo *display.Info, outDir string, test pipTestParams) error {
	a, err := arc.New(ctx, outDir, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to launch ARC")
	}

	const apkName = "ArcPipTest.apk"
	if err := a.Install(ctx, arc.APKPath(apkName)); err != nil {
		return errors.Wrap(err, "failed installing PiP app")
	}

	pipAct, err := arc.NewActivity(a, arcPipTestPkgName, ".PipActivity")
	if err != nil {
		return errors.Wrap(err, "failed to create PiP activity")
	}
	defer pipAct.Close(ctx)

	if err := pipAct.Start(ctx, tconn); err != nil {
		return errors.Wrapf(err, "failed to start %s", pipAct.ActivityName())
	}

	if err := ash.WaitForVisible(ctx, tconn, pipAct.PackageName()); err != nil {
		return errors.Wrap(err, "failed to wait for PiP activity to be visible")
	}

	window, err := ash.GetARCAppWindowInfo(ctx, tconn, pipAct.PackageName())
	if err != nil {
		return errors.Wrapf(err, "failed to get ARC window infomation for package name %s", pipAct.ActivityName())
	}

	// The window is minimized here, but the expected state is PiP, so the async API must used.
	if _, err := ash.SetWindowState(ctx, tconn, window.ID, ash.WMEventMinimize, false /* waitForStateChange */); err != nil {
		return errors.Wrapf(err, "failed to minimize %s", pipAct.ActivityName())
	}

	if err := waitForArcPipWindow(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait for PiP window")
	}

	if err := waitForArcPipWindow(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to enter PiP mode")
	}

	return nil
}

func createBrowserPip(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, ac *uiauto.Context, dataFS http.FileSystem, test pipTestParams) (func(ctx context.Context) error, error) {
	var pipClassName string

	switch test.browserType {
	case browser.TypeAsh:
		pipClassName = "PictureInPictureWindow"
	case browser.TypeLacros:
		pipClassName = "Widget"
	}

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
	pipWindow := nodewith.Name("Picture in picture").ClassName(pipClassName).Onscreen().First()

	if err := action.Combine(
		"show PiP window",
		ac.LeftClick(pipButton),
		ac.WithTimeout(10*time.Second).WaitUntilExists(pipWindow),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to show the PiP window")
	}

	return closeBrowser, nil
}

// waitForArcPipWindow keeps looking for a PiP window until it appears on the Chrome side.
func waitForArcPipWindow(ctx context.Context, tconn *chrome.TestConn) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return w.State == ash.WindowStatePIP
		})

		if err != nil {
			return errors.Wrap(err, "the PiP window hasn't been created yet")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}
