// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type cameraOption struct {
	facing       cca.Facing
	aspectRatios []float64
}

func init() {
	// TODO(b/238157101): We are not running this test on any bots intentionally.
	testing.AddTest(&testing.Test{
		Func:         ImageCaptureWithAspectRatio,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify Image capture with maximum resolution and different aspect ratios (user facing, world facing camera)",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinOrVividCamera},
		Timeout:      5 * time.Minute,
		Fixture:      "ccaLaunched",
		Attr:         []string{"group:intel-nda"},
		Params: []testing.Param{{
			Name: "front",
			Val: cameraOption{facing: cca.FacingFront,
				aspectRatios: []float64{1.3333 /* 4:3 */, 1.7778 /* 16:9 */}},
		}, {
			Name: "back",
			Val: cameraOption{facing: cca.FacingBack,
				aspectRatios: []float64{1.3333 /* 4:3 */, 1.7778 /* 16:9 */}},
		}},
	})
}

func ImageCaptureWithAspectRatio(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	facing := s.Param().(cameraOption).facing
	aspectRatios := s.Param().(cameraOption).aspectRatios

	if facing == cca.FacingBack {
		numCameras, err := app.GetNumOfCameras(ctx)
		if err != nil {
			s.Fatal("Failed to get number of cameras: ", err)
		}
		// since DUT has single camera failing world-facing-camera test.
		if numCameras <= 1 {
			s.Fatal("Failed as DUT does not have Environment Camera")
		}
	}

	if curFacing, err := app.GetFacing(ctx); err != nil {
		s.Fatal("Failed to get facing: ", err)
	} else if curFacing != facing {
		// By default the camera launches the front facing camera.
		// Hence not restoring it at the end of the test.
		if err := app.SwitchCamera(ctx); err != nil {
			s.Fatal("Failed to switch camera: ", err)
		}
		if err := app.CheckFacing(ctx, facing); err != nil {
			s.Fatalf("Failed to switch to the target camera %v: %v", facing, err)
		}
	}

	for _, aspectRatio := range aspectRatios {
		s.Log("Setting aspect ratio: ", aspectRatio)
		if err := setAspectRatio(ctx, app, aspectRatio); err != nil {
			s.Fatal("Failed to set aspect ratio: ", err)
		}
		if err := app.WaitForVideoActive(ctx); err != nil {
			s.Fatal("Failed to wait for video to active: ", err)
		}
		//Capturing 10 images with an aspect ratio.
		for i := 0; i < 10; i++ {
			if err := captureAndVerifyEXIF(ctx, app); err != nil {
				s.Fatal("Failed to capture image and verify EXIF: ", err)
			}
		}
	}

}

// walker type walks through exif fields.
type walker struct{}

// Walk to traverse all the EXIF fields.
func (w walker) Walk(name exif.FieldName, tag *tiff.Tag) error {
	return nil
}

// captureAndVerifyEXIF captures image and verifies its EXIF tags.
func captureAndVerifyEXIF(ctx context.Context, app *cca.App) error {
	fileInfo, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
	if err != nil {
		return errors.Wrap(err, "failed to capture picture")
	}

	if len(fileInfo) > 0 {
		path, err := app.FilePathInSavedDir(ctx, fileInfo[0].Name())
		if err != nil {
			return errors.Wrap(err, "failed to get file path in saved path")
		}

		// Read meta data info from exif for the captured image.
		// Example Image length, width, model name, DateTime, etc,.
		file, err := os.Open(path)
		if err != nil {
			return errors.Wrap(err, "failed to open file")
		}

		defer file.Close()
		data, err := exif.Decode(file)
		if err != nil {
			return errors.Wrap(err, "failed to decode exif")
		}
		var w walker
		// Walk calls the Walk method of w with the name and tag for every non-nil EXIF field.
		// If w aborts the walk with an error, that error is returned.
		if err := data.Walk(w); err != nil {
			return errors.Wrap(err, "failed to read exif data of captured image")
		}
	}
	return nil
}

// setAspectRatio sets aspect ratios(1.333 (4:3), 1.7778 (16:9)) using cca app.
func setAspectRatio(ctx context.Context, app *cca.App, aspectRatio float64) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := app.OpenSettingMenu(ctx, cca.MainMenu); err != nil {
		return errors.Wrap(err, "failed to open main menu")
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.MainMenu)

	if err := app.OpenSettingMenu(ctx, cca.PhotoAspectRatioMenu); err != nil {
		return errors.Wrap(err, "failed to open aspect ratio main menu")
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.PhotoAspectRatioMenu)

	facing, err := app.GetFacing(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get facing")
	}

	aspectRatioOptions := cca.FrontAspectRatioOptions
	if facing == cca.FacingBack {
		aspectRatioOptions = cca.BackAspectRatioOptions
	}

	numOptions, err := app.CountUI(ctx, aspectRatioOptions)
	if err != nil {
		return errors.Wrap(err, "failed to count the aspect ratio options")
	}
	if numOptions < 2 {
		// Ensure that at least two options are provided since "square" will
		// always be an option.
		return errors.Wrapf(err, "unexpected amount of options: %v", numOptions)
	}
	for index := 0; index < numOptions; index++ {
		value, err := app.AttributeWithIndex(ctx, aspectRatioOptions, index, "data-aspect-ratio")
		if err != nil {
			return errors.Wrap(err, "failed to get attribute")
		}

		ar, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return errors.Wrapf(err, "failed to convert aspect ratio value %v to float", aspectRatio)
		}
		// aspectRatioTolerance is the small aspect ratio comparison tolerance.
		const aspectRatioTolerance = 0.02
		if math.Abs(aspectRatio-ar) < aspectRatioTolerance {
			if err := app.ClickWithIndex(ctx, aspectRatioOptions, index); err != nil {
				return errors.Wrap(err, "failed to click on aspect ratio item")
			}
			break
		}
	}
	return nil
}
