// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SurfaceInsets,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test to handle SurfaceInsets not to exceed android window frame",
		Contacts:     []string{"arc-framework+tast@google.com", "hirokisato@google.com"},
		// ChromeOS > Software > ARC++ > Framework > Window Management
		BugComponent: "b:537272",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}},
	})
}

func SurfaceInsets(ctx context.Context, s *testing.State) {
	// Reserve 10 seconds for cleaning up state
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	p := s.FixtValue().(*arc.PreData)
	cr := p.Chrome
	a := p.ARC

	const (
		apk = "ArcSurfaceInsetsTest.apk"
		pkg = "org.chromium.arc.testapp.surfaceinsets"
		cls = ".MainActivity"
	)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure the device is in clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Prepare TouchScreen.
	// Touchscreen bounds: The size of the touchscreen might not be the same
	// as the display size.
	tsw, err := input.Touchscreen(ctx)
	if err != nil {
		s.Fatal("Failed to open touchscreen device: ", err)
	}
	defer tsw.Close(ctx)

	stw, err := tsw.NewSingleTouchWriter()
	if err != nil {
		s.Fatal("Could not get a new TouchEventWriter: ", err)
	}
	defer stw.Close()

	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	act, err := arc.NewActivity(a, pkg, cls)
	if err != nil {
		s.Fatal("Failed to create new activity: ", err)
	}
	defer act.Close(ctx)

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed start Settings activity: ", err)
	}

	if err := act.SetWindowState(ctx, tconn, arc.WindowStateNormal); err != nil {
		s.Fatal("Failed to set window state to Normal: ", err)
	}

	if err := ash.WaitForARCAppWindowState(ctx, tconn, act.PackageName(), ash.WindowStateNormal); err != nil {
		s.Fatal("Failed to wait for window state Normal: ", err)
	}

	windowInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, act.PackageName())
	if err != nil {
		s.Fatal("Failed to get window info: ", err)
	}

	displayInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain a default display: ", err)
	}
	tcc := tsw.NewTouchCoordConverter(displayInfo.Bounds.Size())

	// Repeat twice to validate b/80441010.
	for _, op := range []struct {
		op    string
		state ash.WindowStateType
	}{
		{"maximize", ash.WindowStateMaximized},
		{"restore", ash.WindowStateNormal},
		{"maximize", ash.WindowStateMaximized},
		{"restore", ash.WindowStateNormal},
	} {
		s.Logf("Pressing %q", op)

		// Touch lower edge of the restore/maximize button to validate that any surface does not cover the caption button.
		// TODO(crbug.com/1005010) : Do not hard code the calculation of caption position below.
		// Instead, we should get Chrome constants in real time.
		buttonCoordX, buttonCoordY := tcc.ConvertLocation(coords.NewPoint(
			windowInfo.TargetBounds.Left+windowInfo.TargetBounds.Width-windowInfo.CaptionHeight/2-windowInfo.CaptionHeight,
			windowInfo.TargetBounds.Top+windowInfo.CaptionHeight-windowInfo.CaptionHeight/10))

		stw.Move(buttonCoordX, buttonCoordY)
		stw.End()

		if err := ash.WaitForARCAppWindowState(ctx, tconn, act.PackageName(), ash.WindowStateNormal); err != nil {
			s.Fatalf("Pressing %q did not work: tried to touch (%d, %d): %v", op, buttonCoordX, buttonCoordY, err)
		}
	}

	defer act.Stop(cleanupCtx, tconn)
}
