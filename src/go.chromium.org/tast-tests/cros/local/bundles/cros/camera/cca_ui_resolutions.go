// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"image/jpeg"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/abema/go-mp4"
	"github.com/rwcarlsen/goexif/exif"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIResolutions,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies resolution settings are applied correctly",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome", "arc_camera3"},
		// Default timeout (i.e. 2 minutes) is not enough for some devices to
		// exercise all resolutions on all cameras.
		Timeout:      5 * time.Minute,
		BugComponent: "b:978428",
		Params: []testing.Param{{
			Name:              "real",
			ExtraSoftwareDeps: []string{caps.BuiltinOrVividCamera},
			Fixture:           "ccaTestBridgeReady",
			ExtraAttr:         []string{"group:intel-nda"},
		}, {
			Name:      "fake_hal",
			Fixture:   "ccaTestBridgeReadyWithFakeHALCamera",
			ExtraAttr: []string{"group:criticalstaging"},
		}},
	})
}

// CCAUIResolutions verifies resolution settings for CCA.
func CCAUIResolutions(ctx context.Context, s *testing.State) {
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	subTestTimeout := 2 * time.Minute
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *cca.App) error
	}{{
		"testPhotoResolution",
		testPhotoResolutionAndAspectRatio,
	}, {
		"testVideoResolution",
		testVideoResolutionAndFPS,
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				return tst.testFunc(ctx, app)
			}, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}
}

// getOrientedResolution gets resolution with respect to screen orientation.
func getOrientedResolution(ctx context.Context, app *cca.App, r cca.Resolution) (cca.Resolution, error) {
	orientation, err := app.GetScreenOrientation(ctx)
	if err != nil {
		return r, err
	}
	isLandscape := (orientation == cca.LandscapePrimary || orientation == cca.LandscapeSecondary)
	if isLandscape != (r.Width > r.Height) {
		r.Width, r.Height = r.Height, r.Width
	}
	testing.ContextLogf(ctx, "Screen orientation %v, resolution after orientation %dx%d", orientation, r.Width, r.Height)
	return r, nil
}

func imageResolution(path string, handleOrientation bool) (*cca.Resolution, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open captured file for decoding jpeg")
	}
	defer f.Close()
	c, err := jpeg.DecodeConfig(f)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode captured file")
	}

	if handleOrientation {
		// Read display rotation number from exif.
		f2, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open captured file for reading exif")
		}
		defer f2.Close()
		x, err := exif.Decode(f2)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode exif")
		}
		tag, err := x.Get(exif.Orientation)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get orientation from exif")
		}
		o, err := tag.Int(0)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get integer value of orientation tag")
		}
		if 5 <= o && o <= 8 {
			return &cca.Resolution{Width: c.Height, Height: c.Width}, nil
		}
	}
	return &cca.Resolution{Width: c.Width, Height: c.Height}, nil
}

// takePhotoAndGetResolution takes a photo and extract the resolution of the taken photo
func takePhotoAndGetResolution(ctx context.Context, app *cca.App, handleOrientation bool) (*cca.Resolution, error) {
	info, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
	if err != nil {
		return nil, errors.Wrap(err, "failed to take photo")
	}
	path, err := app.FilePathInSavedDir(ctx, info[0].Name())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get file path")
	}
	ir, err := imageResolution(path, handleOrientation)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get image resolution")
	}
	return ir, nil
}

// recordVideoAndGetResolution records a video and extract the resolution of the recoreded video
func recordVideoAndGetResolution(ctx context.Context, app *cca.App) (*cca.Resolution, error) {
	info, err := app.RecordVideo(ctx, cca.TimerOff, time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to record video")
	}
	path, err := app.FilePathInSavedDir(ctx, info.Name())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get file path")
	}
	vr, err := videoTrackResolution(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract resolution from the video")
	}
	return vr, nil
}

// videoTrackResolution returns the resolution from video file under specified path.
func videoTrackResolution(path string) (*cca.Resolution, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open video file %v", path)
	}
	defer file.Close()

	boxes, err := mp4.ExtractBoxWithPayload(
		file, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeTkhd()})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find track boxes in video file %v", path)
	}
	for _, b := range boxes {
		thkd := b.Payload.(*mp4.Tkhd)
		if thkd.Width > 0 && thkd.Height > 0 {
			// Ignore the 16 low bits of fractional part.
			intW := int(thkd.Width >> 16)
			intH := int(thkd.Height >> 16)

			// All possible rotation matrices(values in matrices are 32-bit fixed-point) in mp4 thkd produced from CCA.
			rotate0 := [9]int32{65536, 0, 0, 0, 65536, 0, 0, 0, 1073741824}
			rotate90 := [9]int32{0, 65536, 0, -65536, 0, 0, 0, 0, 1073741824}
			rotate180 := [9]int32{-65536, 0, 0, 0, -65536, 0, 0, 0, 1073741824}
			rotate270 := [9]int32{0, -65536, 0, 65536, 0, 0, 0, 0, 1073741824}
			switch thkd.Matrix {
			case rotate0:
			case rotate180:
			case rotate90:
				fallthrough
			case rotate270:
				intW, intH = intH, intW
			default:
				return nil, errors.Errorf("unknown mp4 thkd matrix %v", thkd.Matrix)
			}
			return &cca.Resolution{Width: intW, Height: intH}, nil
		}
	}
	return nil, errors.Errorf("no video track found in the file %v", path)
}

// selectOptionAndWaitConfiguration selects the |optionUIName| and waits for the
// camera to be configured. To select the option, we need to open the |menu|
// first and let the menu close either automatically (after choosing the option)
// or manually close it if the option is already checked.
func selectOptionAndWaitConfiguration(ctx context.Context, app *cca.App, subMenu cca.SettingMenu, optionUIName cca.UIComponentName, index int) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	// Opens the setting menu.
	if err := app.OpenSettingMenu(ctx, cca.MainMenu); err != nil {
		return err
	}
	defer app.CloseSettingMenuIfVisible(cleanupCtx, cca.MainMenu)
	if err := app.OpenSettingMenu(ctx, subMenu); err != nil {
		return err
	}
	defer app.CloseSettingMenuIfVisible(cleanupCtx, subMenu)

	// Clicks options and wait for camera configuration.
	testing.ContextLogf(ctx, "Switch to #%v of %v", index, optionUIName)
	checked, err := app.IsCheckedWithIndex(ctx, optionUIName, index)
	if err != nil {
		return err
	}
	if checked {
		testing.ContextLogf(ctx, "#%d resolution option is already checked", index)
	} else {
		if err := app.TriggerConfiguration(ctx, func() error {
			testing.ContextLogf(ctx, "Checking with #%d resolution option", index)
			if err := app.ClickWithIndex(ctx, optionUIName, index); err != nil {
				return errors.Wrap(err, "failed to click on resolution item")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "camera configuration failed after switching resolution")
		}
	}

	return nil
}

// attributeValueOfOption returns the attribute value of |index| th of
// |optionsUIName| given by the name of the attribute. The value will be in integer.
func attributeValueOfOption(ctx context.Context, app *cca.App, optionsUIName cca.UIComponentName, index int, attribute string) (int, error) {
	stringValue, err := app.AttributeWithIndex(ctx, optionsUIName, index, attribute)
	if err != nil {
		return -1, err
	}
	intValue, err := strconv.Atoi(stringValue)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to convert the value (%v) of the attribute (%v) to int", stringValue, attribute)
	}
	return intValue, nil
}

// getOrientedAspectRatio gets aspect ratio with respect to screen orientation.
func getOrientedAspectRatio(ctx context.Context, app *cca.App, aspectRatio float64) (float64, error) {
	orientation, err := app.GetScreenOrientation(ctx)
	if err != nil {
		return 0, err
	}
	isLandscape := (orientation == cca.LandscapePrimary || orientation == cca.LandscapeSecondary)
	if (isLandscape && aspectRatio < 1) || (!isLandscape && aspectRatio > 1) {
		aspectRatio = 1 / aspectRatio
	}
	return aspectRatio, nil
}

// testPhotoResolutionAndAspectRatio tests the mutual behavior of photo resolutions and aspect ratio.
func testPhotoResolutionAndAspectRatio(ctx context.Context, app *cca.App) error {
	return app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		if facing == cca.FacingExternal {
			return nil
		}

		if err := app.SwitchMode(ctx, cca.Photo); err != nil {
			return errors.Wrap(err, "failed to switch to photo mode")
		}

		aspectRatioOptions := cca.FrontAspectRatioOptions
		if facing == cca.FacingBack {
			aspectRatioOptions = cca.BackAspectRatioOptions
		}
		numOptions, err := app.CountUI(ctx, aspectRatioOptions)
		if err != nil {
			return errors.Wrap(err, "failed to count the aspect ratio options")
		} else if numOptions < 2 {
			// Ensure that at least two options are provided since "square" will
			// always be an option.
			return errors.Wrapf(err, "unexpected amount of options: %v", numOptions)
		}

		// Checks that when changing the aspect ratio preference of current
		// running camera, the camera stream will be reconfigured.
		for index := 0; index < numOptions; index++ {
			if err := selectOptionAndWaitConfiguration(ctx, app, cca.PhotoAspectRatioMenu, aspectRatioOptions, index); err != nil {
				return errors.Wrap(err, "failed to click the aspect ratio option and wait for the configration done")
			}

			value, err := app.AttributeWithIndex(ctx, aspectRatioOptions, index, "data-aspect-ratio")
			if err != nil {
				return err
			}
			aspectRatio, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to convert aspect ratio value %v to float", aspectRatio)
			}

			// For "Others" aspect ratio (value is 0.0), since we don't know
			// which resolution will it eventually fallback to, we don't know
			// what is the expected aspect ratio thus we just skip the related
			// check.
			checkAspectRatio := aspectRatio != float64(0)
			if checkAspectRatio {
				aspectRatio, err = getOrientedAspectRatio(ctx, app, aspectRatio)
				if err != nil {
					return errors.Wrap(err, "failed to get oriented aspect ratio value")
				}
			}

			// For each aspect ratio, tests through all possible photo resolution options in the photos resolution settings.
			if err := clickThroughAllPhotoResolutionOptions(ctx, app, facing, checkAspectRatio, aspectRatio); err != nil {
				return errors.Wrap(err, "failed to check photo resolution options")
			}
		}
		return nil
	})
}

// checkResolutionAspectRatio checks the aspect ratio of |resolution| is close to the given |aspectRatio|.
func checkResolutionAspectRatio(ctx context.Context, resolution *cca.Resolution, aspectRatio float64) error {
	// aspectRatioTolerance is the small aspect ratio comparison tolerance for
	// judging the treeya's special resolution 848:480(1.766...) which should be
	// counted as 16:9(1.77...).
	const aspectRatioTolerance = 0.02
	if math.Abs(resolution.AspectRatio()-aspectRatio) > aspectRatioTolerance {
		return errors.Errorf("inconsistent aspect ratio. Resolution: %d:%d; Want aspect ratio: %v", resolution.Width, resolution.Height, aspectRatio)
	}
	return nil
}

// clickThroughAllPhotoResolutionOptions tries out all the photo resolution options under current aspect ratio and ensures the configuration works successfully.
func clickThroughAllPhotoResolutionOptions(ctx context.Context, app *cca.App, facing cca.Facing, checkAspectRatio bool, aspectRatio float64) error {
	photoResolutionOptions := cca.FrontPhotoResolutionOptions
	if facing == cca.FacingBack {
		photoResolutionOptions = cca.BackPhotoResolutionOptions
	}

	numOptions, err := app.CountUI(ctx, photoResolutionOptions)
	if err != nil {
		return errors.Wrap(err, "failed to count the aspect ratio options")
	}

	for index := 0; index < numOptions; index++ {
		if err := selectOptionAndWaitConfiguration(ctx, app, cca.PhotoResolutionMenu, photoResolutionOptions, index); err != nil {
			return errors.Wrap(err, "failed to click the aspect ratio option and wait for the configration done")
		}

		// Ensure preview viewport has correct aspect ratio.
		pr, err := app.GetPreviewViewportSize(ctx)
		if err != nil {
			return err
		}
		if checkAspectRatio {
			if err := checkResolutionAspectRatio(ctx, &pr, aspectRatio); err != nil {
				return err
			}
		}

		// Ensure captured photo has correct aspect ratio.
		handleOrientation := checkAspectRatio && aspectRatio != float64(1)
		ir, err := takePhotoAndGetResolution(ctx, app, handleOrientation)
		if err != nil {
			return err
		}
		if checkAspectRatio {
			if err := checkResolutionAspectRatio(ctx, ir, aspectRatio); err != nil {
				return err
			}
		}
	}
	return nil
}

// testVideoResolutionAndFPS tests the behavior of selecting video resolution and the FPS buttons.
func testVideoResolutionAndFPS(ctx context.Context, app *cca.App) error {
	// The FPS buttons are currently available on external cameras only. Add
	// corresponding tests once they are supported on built-in cameras.
	return app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		if facing == cca.FacingExternal {
			return nil
		}

		if err := app.SwitchMode(ctx, cca.Video); err != nil {
			return errors.Wrap(err, "failed to switch to video mode")
		}

		videoResolutionOptions := cca.FrontVideoResolutionOptions
		if facing == cca.FacingBack {
			videoResolutionOptions = cca.BackVideoResolutionOptions
		}
		numOptions, err := app.CountUI(ctx, videoResolutionOptions)
		if err != nil {
			return errors.Wrap(err, "failed to count the video resolution options")
		}

		for index := 0; index < numOptions; index++ {
			if err := selectOptionAndWaitConfiguration(ctx, app, cca.VideoResolutionMenu, videoResolutionOptions, index); err != nil {
				return errors.Wrap(err, "failed to click the aspect ratio option and wait for the configration done")
			}

			width, err := attributeValueOfOption(ctx, app, videoResolutionOptions, index, "data-width")
			if err != nil {
				return err
			}
			height, err := attributeValueOfOption(ctx, app, videoResolutionOptions, index, "data-height")
			if err != nil {
				return err
			}

			shouldCheckResolution := (width == 3840 && height == 2160) ||
				(width == 2560 && height == 1440) ||
				(width == 1920 && height == 1080) ||
				(width == 1280 && height == 720)
			if !shouldCheckResolution {
				continue
			}

			r := cca.Resolution{Width: width, Height: height}
			or, err := getOrientedResolution(ctx, app, r)
			if err != nil {
				return errors.Wrap(err, "failed to get oriented resolution")
			}
			// Ensure preview viewport has correct aspect ratio as the target video resolution.
			pr, err := app.GetPreviewViewportSize(ctx)
			if err != nil {
				return err
			}
			if err := checkResolutionAspectRatio(ctx, &pr, or.AspectRatio()); err != nil {
				return err
			}

			// Ensure captured video has correct resolution.
			vr, err := recordVideoAndGetResolution(ctx, app)
			if err != nil {
				return err
			}
			if vr.Width != or.Width || vr.Height != or.Height {
				return errors.Wrapf(err, "incorrect captured resolution get %dx%d; want %dx%d", vr.Width, vr.Height, or.Width, or.Height)
			}
		}
		return nil
	})
}
