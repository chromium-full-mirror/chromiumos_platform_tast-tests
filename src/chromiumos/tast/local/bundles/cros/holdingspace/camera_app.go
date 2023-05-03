// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"
	"time"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/camera/cca"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast/core/testing"
)

type cameraAppParams struct {
	testfunc func(*cca.App, *string) uiauto.Action
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies Camera app integrations with holding space",
		BugComponent: "b:1268276", // ChromeOS > Software > System UI Surfaces > HoldingSpace
		Contacts: []string{
			"tote-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"dmblack@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "ccaLaunchedWithHoldingSpaceIntegrationEnabled",
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinCamera},
		Params: []testing.Param{{
			Name: "photo",
			Val: cameraAppParams{
				testfunc: testCameraAppPhoto,
			},
		}, {
			Name: "video_gif",
			Val: cameraAppParams{
				testfunc: testCameraAppVideoGif,
			},
		}, {
			Name: "video_mp4",
			Val: cameraAppParams{
				testfunc: testCameraAppVideoMp4,
			},
		}},
	})
}

// CameraApp verifies Camera app integrations with holding space. It is expected
// that taking a photo or a video will result in an item being added to holding
// space.
func CameraApp(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	cr := s.FixtValue().(cca.FixtureData).Chrome

	s.FixtValue().(cca.FixtureData).
		SetDebugParams(cca.DebugParams{SaveScreenshotWhenFail: true})

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	ui := uiauto.New(tconn)

	var fileName string
	if err := uiauto.Combine("take photo/video",
		// Take a photo/video per test parameterization.
		s.Param().(cameraAppParams).testfunc(app, &fileName),

		// Left click the tray to open the bubble.
		ui.LeftClick(holdingspace.FindTray()),

		// Verify the existence of the photo/video in holding space. Note that this
		// action is wrapped so that `fileName` is resolved only after the photo/
		// video has been taken.
		func(ctx context.Context) error {
			return ui.WaitUntilExists(
				holdingspace.FindDownloadChip().Name(fileName))(ctx)
		},
	)(ctx); err != nil {
		s.Fatal("Failed to take photo/video: ", err)
	}
}

// testCameraAppPhoto performs testing of taking a photo.
func testCameraAppPhoto(app *cca.App, fileName *string) uiauto.Action {
	return switchCameraAppModeAnd(
		app, cca.Photo, func(ctx context.Context) error {
			fileInfo, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
			if err == nil && len(fileInfo) > 0 {
				*fileName = fileInfo[0].Name()
			}
			return err

		})
}

// testCameraAppVideoGif performs testing of taking a GIF video.
func testCameraAppVideoGif(app *cca.App, fileName *string) uiauto.Action {
	return switchCameraAppModeAnd(
		app, cca.Video, func(ctx context.Context) error {
			fileInfo, err := app.RecordGif(ctx /*save=*/, true)
			if err == nil {
				*fileName = fileInfo.Name()
			}
			return err
		})
}

// testCameraAppVideoMp4 performs testing of taking an MP4 video.
func testCameraAppVideoMp4(app *cca.App, fileName *string) uiauto.Action {
	return switchCameraAppModeAnd(
		app, cca.Video, func(ctx context.Context) error {
			fileInfo, err := app.RecordVideo(ctx, cca.TimerOff, 1*time.Second)
			if err == nil {
				*fileName = fileInfo.Name()
			}
			return err
		})
}

// switchCameraAppModeAnd switches the Camera `app` to the specified `mode` and,
// if successful, executes the supplied `action`.
func switchCameraAppModeAnd(
	app *cca.App, mode cca.Mode, action uiauto.Action) uiauto.Action {
	return func(ctx context.Context) error {
		if err := app.SwitchMode(ctx, mode); err != nil {
			return err
		}
		return action(ctx)
	}
}
