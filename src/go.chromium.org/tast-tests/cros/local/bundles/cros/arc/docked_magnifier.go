// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"image/color"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/media/imgcmp"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DockedMagnifier,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks DockedMagnifier works for arc windows as expected",
		Contacts:     []string{"arc-framework+tast@google.com", "hirokisato@chromium.org", "yaoqq@chromium.org"},
		BugComponent: "b:153260",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"group:hw_agnostic"},
		}},
	})
}

func DockedMagnifier(ctx context.Context, s *testing.State) {
	const (
		dmApkName          = "ArcDockedMagnifierTest.apk"
		dmPackageName      = "org.chromium.arc.testapp.dockedmagnifiertest"
		dmMainActivityName = dmPackageName + ".MainActivity"
	)

	d := s.FixtValue().(*arc.PreData)
	a := d.ARC
	cr := d.Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := a.Install(ctx, arc.APKPath(dmApkName)); err != nil {
		s.Fatal("Failed to install the app: ", err)
	}

	activity, err := arc.NewActivity(a, dmPackageName, dmMainActivityName)
	if err != nil {
		s.Fatal("Failed to create new activity: ", err)
	}

	if err := activity.Start(ctx, tconn); err != nil {
		s.Fatal("Failed to start activity: ", err)
	}

	pc, err := pointer.NewTouch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create a touch controller: ", err)
	}
	defer pc.Close(ctx)

	if err := testMaximizedWindow(ctx, tconn, cr, activity, pc); err != nil {
		s.Error("Failed to run test for maximized window: ", err)
	}
}

func testMaximizedWindow(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, activity *arc.Activity, pc pointer.Context) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Maximize the window
	if _, err := ash.SetARCAppWindowStateAndWait(ctx, tconn, activity.PackageName(), ash.WindowStateMaximized); err != nil {
		return errors.Wrap(err, "failed to set window state to maximized")
	}
	if err := checkWindowBoundsAndContentForMaximizedWindow(ctx, tconn, cr, activity, pc); err != nil {
		return errors.Wrap(err, "failed to verify window bounds and content resize properly after maximizing the window")
	}

	// Enable DockedMagnifier
	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.DockedMagnifier, true); err != nil {
		return errors.Wrap(err, "failed to enable Docked Magnifier feature")
	}
	defer a11y.ClearFeature(cleanupCtx, tconn, a11y.DockedMagnifier)
	if err := checkWindowBoundsAndContentForMaximizedWindow(ctx, tconn, cr, activity, pc); err != nil {
		return errors.Wrap(err, "failed to verify window bounds and content resize properly after enabling Docked Magnifier")
	}

	// Disable DockedMagnifier
	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.DockedMagnifier, false); err != nil {
		return errors.Wrap(err, "failed to disable Docked Magnifier feature")
	}
	if err := checkWindowBoundsAndContentForMaximizedWindow(ctx, tconn, cr, activity, pc); err != nil {
		return errors.Wrap(err, "failed to verify window bounds and content resize properly after disabling Docked Magnifier")
	}

	return nil
}

// checkWindowBoundsAndContentForMaximizedWindow checks if the bounds of a maximized window is the same as workArea and the content of it is properly rendered.
func checkWindowBoundsAndContentForMaximizedWindow(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, activity *arc.Activity, pc pointer.Context) error {
	const (
		coloredPixelPercentThreshold = 97
		boundsComparingMargin        = 1
	)

	primaryDisplayInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get primary display info")
	}
	displayMode, err := ash.PrimaryDisplayMode(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get display mode of the primary display")
	}

	var bounds coords.Rect
	var captionHeight int

	// In tablet mode, the caption bar is auto-hidden. But sometimes it's shown due to cursor at the top of the window.
	// Here we perform a click in the center of the window(same bounds as the WorkArea) to make sure that the caption bar is hidden.
	if err := pc.ClickAt(coords.NewPoint(primaryDisplayInfo.WorkArea.CenterX(), primaryDisplayInfo.WorkArea.CenterY()))(ctx); err != nil {
		return errors.Wrap(err, "failed to click the center of WorkArea to ensure no caption bar for tablet mode")
	}

	// Check if window bounds is the same as workArea(when docked magnifier is enabled, the workArea is the bottom part of the display below the magnifier).
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		windowInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, activity.PackageName())
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get arc app window info"))
		}

		bounds = windowInfo.TargetBounds
		if !coords.CompareBoundsWithMargin(bounds, primaryDisplayInfo.WorkArea, boundsComparingMargin) {
			return errors.Errorf("invalid window bounds comparing to display work area, window: %s, work area: %s", bounds, primaryDisplayInfo.WorkArea)
		}

		tabletMode, err := ash.TabletModeEnabled(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to check if table mode is enabled")
		}
		// When it's in tabletMode there should not be caption.
		if tabletMode {
			captionHeight = 0
		} else {
			captionHeight = windowInfo.CaptionHeight
		}

		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return err
	}

	boundsWithoutCaption := coords.NewRectLTRB(
		bounds.Left,
		bounds.Top+captionHeight,
		bounds.Right(),
		bounds.Bottom(),
	)
	boundsWithoutCaptionPx := coords.ConvertBoundsFromDPToPX(
		boundsWithoutCaption,
		displayMode.DeviceScaleFactor,
	)
	blueBoundsPx := coords.NewRect(
		boundsWithoutCaptionPx.Left,
		boundsWithoutCaptionPx.Top,
		boundsWithoutCaptionPx.Width,
		boundsWithoutCaptionPx.Height/2,
	)
	redBoundsPx := coords.NewRect(
		boundsWithoutCaptionPx.Left,
		boundsWithoutCaptionPx.Top+boundsWithoutCaptionPx.Height/2,
		boundsWithoutCaptionPx.Width,
		boundsWithoutCaptionPx.Height/2,
	)

	// Check if the window content is properly rendered by checking the percentage of the colored pixels(the upper half of the app has a blue background, while the bottom half has a red background).
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := screenshotAndCheckColoredPercent(ctx, cr, blueBoundsPx, color.RGBA{0, 0, 255, 255}, coloredPixelPercentThreshold); err != nil {
			return errors.Wrap(err, "failed to verify blue pixels")
		}
		if err := screenshotAndCheckColoredPercent(ctx, cr, redBoundsPx, color.RGBA{255, 0, 0, 255}, coloredPixelPercentThreshold); err != nil {
			return errors.Wrap(err, "failed to verify red pixels")
		}

		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return err
	}

	return nil
}

func screenshotAndCheckColoredPercent(ctx context.Context, cr *chrome.Chrome, bounds coords.Rect, clr color.Color, threshold int) error {
	img, err := screenshot.GrabAndCropScreenshot(ctx, cr, bounds)
	if err != nil {
		return err
	}

	totalPixels := bounds.Height * bounds.Width
	coloredPixels := imgcmp.CountPixels(img, clr)
	coloredPercent := coloredPixels * 100 / totalPixels

	if coloredPercent < threshold {
		return errors.Errorf("failed to verify the number of the %v colored pixels exceeds the threshold (%d%%); contains %d / %d (%d%%) colored pixels in bounds %s", clr, threshold, coloredPixels, totalPixels, coloredPercent, bounds)
	}

	return nil
}
