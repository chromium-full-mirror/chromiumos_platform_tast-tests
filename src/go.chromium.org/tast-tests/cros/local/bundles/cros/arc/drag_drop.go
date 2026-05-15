// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type dragDropTestArgs struct {
	androidSource bool
	androidTarget bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     DragDrop,
		Desc:     "Checks drag and drop support from Chrome to ARC",
		Contacts: []string{"arc-framework+tast@google.com", "yhanada@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Chrome Integration
		BugComponent: "b:537221",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"drag_source_window.html", "drag_target_window.html"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Fixture:      "arcBooted",
		Params: []testing.Param{
			{
				Name:              "chrome_to_android_vm",
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Val: &dragDropTestArgs{
					androidSource: false,
					androidTarget: true,
				},
			}, {
				Name:              "android_to_android_vm",
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Val: &dragDropTestArgs{
					androidSource: true,
					androidTarget: true,
				},
			}, {
				Name:              "android_to_chrome_vm",
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Val: &dragDropTestArgs{
					androidSource: true,
					androidTarget: false,
				},
			}},
	})
}

func DragDrop(ctx context.Context, s *testing.State) {
	const (
		sourceApk     = "ArcDragSourceTest.apk"
		sourcePkg     = "org.chromium.arc.testapp.dragsource"
		sourceActName = ".DragSourceActivity"
		targetApk     = "ArcDragTargetTest.apk"
		targetPkg     = "org.chromium.arc.testapp.dragtarget"
		targetActName = ".DragTargetActivity"

		dragAreaViewID = sourcePkg + ":id/drag_area"

		// Title of the test HTML
		pageTitle = "Chrome - DragDrop Controller"

		// Packagename of placeholder activity.
		placeholderPkg = "org.chromium.arc.applauncher"

		// width and height of target and source windows.
		w = 500
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second*10)
	defer cancel()

	args := s.Param().(*dragDropTestArgs)

	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	s.Log("Start the Web server")
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	startActivityWithBounds := func(ctx context.Context, apk, pkg, activityName string, wantBounds coords.Rect) (act *arc.Activity, err error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, time.Second*10)
		defer cancel()

		if err = a.Install(ctx, arc.APKPath(apk)); err != nil {
			err = errors.Wrap(err, "failed installing app")
			return
		}

		if act, err = arc.NewActivity(a, pkg, activityName); err != nil {
			err = errors.Wrap(err, "failed to create a new activity")
			return
		}

		if err = act.StartWithDefaultOptions(ctx, tconn); err != nil {
			act.Close(ctx)
			act = nil
			err = errors.Wrap(err, "failed to start the activity")
			return
		}

		defer func(ctx context.Context) {
			if err != nil {
				act.Stop(cleanupCtx, tconn)
				act.Close(ctx)
				act = nil
			}
		}(cleanupCtx)

		var window *ash.Window
		if window, err = ash.FindWindow(ctx, tconn, func(window *ash.Window) bool {
			return window.ARCPackageName == pkg
		}); err != nil {
			err = errors.Wrap(err, "failed to find the ARC window")
			return
		}

		if err = act.SetWindowState(ctx, tconn, arc.WindowStateNormal); err != nil {
			err = errors.Wrap(err, "failed to set the window state to normal")
			return
		}

		if err = ash.WaitForCondition(ctx, tconn, func(cur *ash.Window) bool {
			return cur.ID == window.ID && cur.State == ash.WindowStateNormal && !cur.IsAnimating
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			err = errors.Wrap(err, "failed to wait for the window to finish animating")
			return
		}

		var gotBounds coords.Rect
		if gotBounds, _, err = ash.SetWindowBounds(ctx, tconn, window.ID, wantBounds, window.DisplayID); err != nil {
			err = errors.Wrap(err, "failed to set window bounds")
			return
		} else if gotBounds != wantBounds {
			err = errors.Errorf("failed to resize the activity: got %v; want %v", gotBounds, wantBounds)
			return
		}
		return
	}

	sourceBounds := coords.Rect{Left: w, Top: 0, Width: w, Height: w}
	targetBounds := coords.Rect{Left: 0, Top: 0, Width: w, Height: w}

	if !args.androidSource {
		pageURL := server.URL + "/drag_source_window.html"
		sourceConn, err := openWebpageWithBounds(ctx, cr, tconn, pageURL, sourceBounds)
		if err != nil {
			s.Fatalf("Could not connect to page at %v: %v", pageURL, err)
		}
		defer sourceConn.Close()
	}
	var targetConn *chrome.Conn
	if !args.androidTarget {
		pageURL := server.URL + "/drag_target_window.html"
		targetConn, err = openWebpageWithBounds(ctx, cr, tconn, pageURL, targetBounds)
		if err != nil {
			s.Fatalf("Could not connect to page at %v: %v", pageURL, err)
		}
		defer targetConn.Close()
	}

	if args.androidSource {
		sourceAct, err := startActivityWithBounds(ctx, sourceApk, sourcePkg, sourceActName, sourceBounds)
		if err != nil {
			s.Fatal("Failed to start an activity with bounds: ", err)
		}
		defer sourceAct.Close(cleanupCtx)
		defer sourceAct.Stop(cleanupCtx, tconn)

		// Makes sure that the drag source View is inflated at a proper location.
		if err := waitForViewInsideBounds(ctx, tconn, d.Object(ui.ID(dragAreaViewID)), sourceBounds); err != nil {
			s.Fatal("Failed to wait for the drag area view: ", err)
		}
	}

	var targetAct *arc.Activity
	if args.androidTarget {
		targetAct, err = startActivityWithBounds(ctx, targetApk, targetPkg, targetActName, targetBounds)
		if err != nil {
			s.Fatal("Failed to start an activity with bounds: ", err)
		}
		defer targetAct.Close(cleanupCtx)
		defer targetAct.Stop(cleanupCtx, tconn)

		if !args.androidSource {
			// This is a workaround to wait for placeholder activity launches.
			//
			// In a drag-and-drop from Chrome to Android, we currently rely on Placeholder Activity.
			// It is started for the first time ARC window goes behind.
			// Thus, if we just inject drag event after starting the activity, there's a race
			// between launching the placeholder activity and starting a drag operation.
			// TODO(b/337997097): We should remove dependency on placeholder.
			if _, err := ash.BringWindowToForeground(ctx, tconn, pageTitle); err != nil {
				s.Fatal("Failed to activate the source app: ", err)
			}

			if err := d.Object(ui.PackageName(placeholderPkg)).WaitForExists(ctx, 10*time.Second); err != nil {
				s.Fatal("Failed to wait for placeholder launched: ", err)
			}
		}
	}

	if err := mouse.Drag(tconn, sourceBounds.CenterPoint(), targetBounds.CenterPoint(), time.Second)(ctx); err != nil {
		s.Fatal("Failed to send drag events: ", err)
	}

	if args.androidTarget {
		if err := targetAct.Focus(ctx, tconn); err != nil {
			s.Fatal("Failed to focus the activity: ", err)
		}

		const (
			expected = `ClipData { text/plain "" {T:Data text} }`
			fieldID  = targetPkg + ":id/dropped_data_view"
		)
		if err := d.Object(ui.ID(fieldID)).WaitForText(ctx, expected, 30*time.Second); err != nil {
			s.Fatal("Failed to wait for the drag and drop result: ", err)
		}
	} else {
		const expected = "Data text"
		script := fmt.Sprintf(`document.getElementById('dropped-data').innerHTML === %q`, expected)
		if err := targetConn.WaitForExprWithTimeout(ctx, script, 30*time.Second); err != nil {
			s.Fatal("Failed to wait for the dropped data: ", err)
		}
	}
}

func waitForViewInsideBounds(ctx context.Context, tconn *chrome.TestConn, uiObj *ui.Object, bounds coords.Rect) error {
	if err := uiObj.WaitForExists(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for view exists")
	}

	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get display info")
	}
	dsf, err := dispInfo.GetEffectiveDeviceScaleFactor()
	if err != nil {
		return errors.Wrap(err, "failed to get display dsf")
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		viewBounds, err := uiObj.GetBounds(ctx)
		if err != nil {
			return err
		}
		viewBoundsDP := coords.ConvertBoundsFromPXToDP(viewBounds, dsf)
		if !bounds.Contains(viewBoundsDP) {
			return errors.Errorf("view bounds is not ready. container=%v, view=%v", bounds, viewBoundsDP)
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second})
}

func openWebpageWithBounds(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, url string, bounds coords.Rect) (*chrome.Conn, error) {
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the page")
	}
	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "failed to wait for the page loaded")
	}
	window, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "failed to get the active window")
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, window.ID, ash.WindowStateNormal); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "failed to set the window state to Normal")
	}
	if _, _, err := ash.SetWindowBounds(ctx, tconn, window.ID, bounds, window.DisplayID); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "failed to set the window bounds")
	}
	return conn, nil
}
