// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIScreenRotate,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Takes pictures in every screen orientation and checks the picture resolutions",
		Contacts:     []string{"chromeos-camera-eng@google.com", "intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Attr:         []string{"group:mainline", "group:camera-libcamera", "informational", "group:intel-nda"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Params: []testing.Param{{
			Name:    "fake",
			Fixture: "ccaLaunchedWithFakeHALCamera",
		}, {
			Name:              "real",
			Fixture:           "ccaLaunched",
			ExtraSoftwareDeps: []string{caps.BuiltinOrVividCamera},
		}},
	})
}

func checkOrientation(facing cca.Facing, screen cca.Orientation, resolution *cca.Resolution) error {
	landscapePic := resolution.Width > resolution.Height
	landscapeScreen := screen == cca.LandscapePrimary || screen == cca.LandscapeSecondary

	if facing == cca.FacingExternal {
		// For external cameras, the taken picture should not be rotated with screen.
		if !landscapePic {
			return errors.New("external camera should have portrait picture")
		}
		return nil
	}

	if landscapeScreen != landscapePic {
		return errors.Errorf("unexpected %dx%d picture when screen orientation is %s",
			resolution.Width, resolution.Height, screen)
	}

	return nil
}

// CCAUIScreenRotate Open CCA, rotate the display to either take
// picture or record video using all available cameras.
func CCAUIScreenRotate(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cr := s.FixtValue().(cca.FixtureData).Chrome
	app := s.FixtValue().(cca.FixtureData).App()
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveScreenshotWhenFail: true})
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	cleanup, err := app.EnsureTabletModeEnabled(ctx, true)
	if err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Get display info.
	dispInfo, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get internal display info: ", err)
	}
	// Revert back to initial screen orientation.
	defer func(ctx context.Context) {
		s.Log("Setting back to initial orientation")
		if err := display.SetDisplayRotationSync(ctx, tconn, dispInfo.ID, display.Rotate0); err != nil {
			s.Fatal("Failed to rotate display 0 degree: ", err)
		}
	}(cleanupCtx)

	app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		s.Logf("Starting test with %v facing camera", facing)

		for _, tc := range []struct {
			name         string
			mode         cca.Mode
			screenOrient cca.Orientation
			dispRotate   display.RotationAngle
		}{
			{"testPhotoRotate0", cca.Photo, cca.LandscapePrimary, display.Rotate0},
			{"testPhotoRotate90", cca.Photo, cca.PortraitPrimary, display.Rotate90},
			{"testPhotoRotate180", cca.Photo, cca.LandscapeSecondary, display.Rotate180},
			{"testPhotoRotate270", cca.Photo, cca.PortraitSecondary, display.Rotate270},
			{"testVideoRotate0", cca.Video, cca.LandscapePrimary, display.Rotate0},
			{"testVideoRotate90", cca.Video, cca.PortraitPrimary, display.Rotate90},
			{"testVideoRotate180", cca.Video, cca.LandscapeSecondary, display.Rotate180},
			{"testVideoRotate270", cca.Video, cca.PortraitSecondary, display.Rotate270},
		} {
			s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
				if err := display.SetDisplayRotationSync(ctx, tconn, dispInfo.ID, tc.dispRotate); err != nil {
					s.Fatalf("Failed to rotate display %v degree: %v", tc.dispRotate, err)
				}

				if err := app.SwitchMode(ctx, tc.mode); err != nil {
					s.Fatal("Failed to switch mode: ", err)
				}

				info := func() os.FileInfo {
					switch tc.mode {
					case cca.Photo:
						info, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
						if err != nil {
							s.Fatal("Failed to take photo: ", err)
						}
						return info[0]
					case cca.Video:
						info, err := app.RecordVideo(ctx, cca.TimerOff, 1*time.Second)
						if err != nil {
							s.Fatal("Failed to record video: ", err)
							return nil
						}
						return info
					default:
						s.Fatal("Unexpected mode: ", tc.mode)
						return nil
					}
				}()
				path, err := app.FilePathInSavedDir(ctx, info.Name())
				if err != nil {
					s.Fatal("Failed to get file path: ", err)
				}
				resolution, err := cca.ExtractResolution(ctx, path)
				if err != nil {
					s.Fatalf("Failed to extract resolution from %s: %v", path, err)
				}

				if err := app.SaveScreenshot(ctx); err != nil {
					s.Error("Failed to save a screenshot: ", err)
				}

				orient, err := app.GetScreenOrientation(ctx)
				if err != nil {
					s.Fatal("Failed to get screen orientation: ", err)
				}
				if orient != tc.screenOrient {
					s.Fatalf("Failed to match screen orientation: got %q; want %q", orient, tc.screenOrient)
				}

				if err = checkOrientation(facing, orient, resolution); err != nil {
					s.Errorf("Failed to check orientation in %v subtest: %v:", tc.name, err)
				}
			})
		}
		return nil
	})
}
