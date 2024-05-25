// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"io/fs"
	"math"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIDocumentScanning,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that CCA can take a photo for document and generate the document file with fake HAL",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome", "ondevice_document_scanner_rootfs_or_dlc", caps.BuiltinOrVividCamera},
		Data:         []string{"document_3264x2448.mjpeg", "ocr_one_line_3264x2448.jpg"},
		Fixture:      "ccaTestBridgeReadyWithFakeHALCameraWithPDFOCR",
	})
}

type documentScanRunSubTest func(ctx context.Context, app *cca.App) error

type documentScanSubTest struct {
	name  string
	run   documentScanRunSubTest
	scene string
}

type docCorner struct {
	x, y float64
}

type docArea struct {
	// corners is coordinates of document corners starts from left-top
	// corner and in counter-clockwise order. Numbers are normalized with
	// width, height of original image(before cropping).
	corners [4]docCorner
}

// checkSimilar checks if two area are similar with tolerance.
func (area *docArea) checkSimilar(area2 *docArea) error {
	const tolerance = 0.1
	for i, corn := range area.corners {
		corn2 := area2.corners[i]
		if math.Abs(corn.x-corn2.x) > tolerance {
			return errors.Errorf("coordindate x mismatch for comparing document area %v and %v", area, area2)
		}
		if math.Abs(corn.y-corn2.y) > tolerance {
			return errors.Errorf("coordindate y mismatch for comparing document area %v and %v", area, area2)
		}
	}
	return nil
}

var (
	// The shorter document on the left of camera scene. Coordinates are
	// derived from equation like the following with chrome developer tool:
	// https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/bd5e4f1ccbc59cc3e4dda6fa71eadedf295fca28/src/go.chromium.org/tast-tests/cros/local/bundles/cros/camera/data/cca_ui.js#207
	doc1Area = &docArea{[4]docCorner{
		{0.05298, 0.44720},
		{0.03376, 0.84603},
		{0.49621, 0.77297},
		{0.46445, 0.39194},
	}}
	// The longer document on the right of camera scene.
	doc2Area = &docArea{[4]docCorner{
		{0.53727, 0.16051},
		{0.56251, 0.88772},
		{0.99996, 0.86272},
		{0.89309, 0.15380},
	}}
)

// CCAUIDocumentScanning is the entry point for local document scanning test.
func CCAUIDocumentScanning(ctx context.Context, s *testing.State) {
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene
	cr := s.FixtValue().(cca.FixtureData).Chrome
	bt := s.FixtValue().(cca.FixtureData).BrowserType
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveCameraFolderWhenFail: true})

	subTestTimeout := 30 * time.Second
	for _, tst := range []documentScanSubTest{{
		name: "testSavePhoto",
		run:  testSavePhoto,
	}, {
		name: "testSavePdf",
		run:  testSavePdf,
	}, {
		name: "testUIChangeWithDifferentPageCount",
		run:  testUIChangeWithDifferentPageCount,
	}, {
		name: "testFixCropArea",
		run: func(ctx context.Context, app *cca.App) error {
			return testFixCropArea(ctx, app, cr)
		},
	}, {
		name: "testPDFOCR",
		run: func(ctx context.Context, app *cca.App) error {
			return testPDFOCR(ctx, app, cr, bt, "hello.", s.OutDir())
		},
		scene: "ocr_one_line_3264x2448.jpg",
	}} {
		s.Run(ctx, tst.name, func(ctx context.Context, s *testing.State) {
			subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
			defer cancel()

			scene := tst.scene
			if scene == "" {
				scene = "document_3264x2448.mjpeg"
			}
			if err := switchScene(ctx, cca.SceneData{Path: s.DataPath(scene)}); err != nil {
				s.Fatal("Failed to prepare document scene: ", err)
			}

			if err := runTestWithApp(subTestCtx, func(subTestCtx context.Context, app *cca.App) error {
				if err := app.EnterDocumentMode(ctx); err != nil {
					return errors.Wrap(err, "failed to enter document mode")
				}
				return tst.run(subTestCtx, app)
			}, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
	}
}

// testSavePhoto tests if CCA can take a document photo and save the file as JPG correctly.
func testSavePhoto(ctx context.Context, app *cca.App) (retErr error) {
	if err := clickShutterAndWaitFor(ctx, app, cca.DocumentReview); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show")
	}

	start := time.Now()

	if err := app.Click(ctx, cca.DocumentSaveAsPhotoButton); err != nil {
		return errors.Wrap(err, "failed to click save as photo button")
	}

	if _, err := waitForFileSaved(ctx, app, cca.DocumentPhotoPattern, start); err != nil {
		return errors.Wrap(err, "failed to wait for the photo")
	}

	return nil
}

// testSavePdf tests if CCA can take document photos and save the file as PDF correctly.
func testSavePdf(ctx context.Context, app *cca.App) (retErr error) {
	if err := clickShutterAndWaitFor(ctx, app, cca.DocumentReview); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show")
	}

	if err := app.Click(ctx, cca.DocumentAddPageButton); err != nil {
		return errors.Wrap(err, "failed to click the add page button")
	}

	if err := app.WaitForState(ctx, "camera-configuring", false); err != nil {
		return errors.Wrap(err, "failed to wait for camera-configuring state to turn off")
	}

	if err := clickShutterAndWaitFor(ctx, app, cca.DocumentReview); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show")
	}

	start := time.Now()

	if err := app.Click(ctx, cca.DocumentSaveAsPdfButton); err != nil {
		return errors.Wrap(err, "failed to click save as PDF button")
	}

	if _, err := waitForFileSaved(ctx, app, cca.DocumentPDFPattern, start); err != nil {
		return errors.Wrap(err, "failed to wait for the PDF file")
	}

	return nil
}

// testUIChangeWithDifferentPageCount tests if CCA shows or hides the UI components correctly during different page counts.
func testUIChangeWithDifferentPageCount(ctx context.Context, app *cca.App) (retErr error) {
	// 1 page
	if err := clickShutterAndWaitFor(ctx, app, cca.DocumentReview); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show")
	}

	if err := app.Click(ctx, cca.DocumentAddPageButton); err != nil {
		return errors.Wrap(err, "failed to click the add page button")
	}

	if err := app.WaitForVisibleState(ctx, cca.DocumentReview, false); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to hide")
	}

	if err := app.Click(ctx, cca.DocumentBackButton); err != nil {
		return errors.Wrap(err, "failed to click the back button")
	}

	if err := app.WaitForVisibleState(ctx, cca.DocumentReview, true); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show")
	}

	if err := app.CheckVisible(ctx, cca.DocumentSaveAsPhotoButton, true); err != nil {
		return errors.Wrap(err, "failed to check visibility of save as photo button")
	}

	if err := app.Click(ctx, cca.DocumentAddPageButton); err != nil {
		return errors.Wrap(err, "failed to click the add page button")
	}

	if err := app.WaitForVisibleState(ctx, cca.DocumentReview, false); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show up")
	}

	if err := app.WaitForState(ctx, "camera-configuring", false); err != nil {
		return errors.Wrap(err, "failed to wait for state camera-configuring to turn off")
	}

	// 2 pages
	if err := clickShutterAndWaitFor(ctx, app, cca.DocumentReview); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show")
	}

	if err := app.CheckVisible(ctx, cca.DocumentSaveAsPhotoButton, false); err != nil {
		return errors.Wrap(err, "failed to check visibility of save as photo button")
	}

	// 0 pages
	if err := app.Click(ctx, cca.DocumentCancelButton); err != nil {
		return errors.Wrap(err, "failed to click the cancel button")
	}

	if err := app.WaitForVisibleState(ctx, cca.DocumentReview, false); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to hide")
	}

	if err := app.CheckVisible(ctx, cca.DocumentBackButton, false); err != nil {
		return errors.Wrap(err, "failed to check visibility of back button")
	}

	return nil
}

func testFixCropArea(ctx context.Context, app *cca.App, cr *chrome.Chrome) error {
	if err := clickShutterAndWaitFor(ctx, app, cca.DocumentReview); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show")
	}

	if visible, err := app.Visible(ctx, cca.DocumentPreviewModeImage); err != nil {
		return errors.Wrap(err, "failed to check the visibility of preview mode image")
	} else if !visible {
		return errors.New("preview mode image is not visible")
	}

	imageElSize, err := app.Size(ctx, cca.DocumentPreviewModeImage)
	if err != nil {
		return errors.Wrap(err, "failed to get review size at initial scan")
	}

	if imageElSize.Width <= imageElSize.Height {
		return errors.Errorf("should crop the shorter document at initial scan, got document width: %v, height: %v", imageElSize.Width, imageElSize.Height)
	}

	if err := app.Click(ctx, cca.DocumentFixButton); err != nil {
		return errors.Wrap(err, "failed to click the fix button")
	}

	if err := app.WaitForVisibleState(ctx, cca.DocumentFixModeImage, true); err != nil {
		return errors.Wrap(err, "failed to wait for fix mode image to show up")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	// Poll the check of positions of corners since fix mode UI may resize after showing.
	err = testing.Poll(ctx, func(ctx context.Context) error {
		var corners [4]docCorner

		imageElSize, err := app.Size(ctx, cca.DocumentFixModeImage)
		if err != nil {
			return testing.PollBreak(err)
		}

		imageElScreenXY, err := app.ScreenXYWithIndex(ctx, cca.DocumentFixModeImage, 0)
		if err != nil {
			return testing.PollBreak(err)
		}

		dotElSize, err := app.Size(ctx, cca.DocumentFixModeCorner)
		if err != nil {
			return testing.PollBreak(err)
		}

		for i := range corners {
			pt, err := app.ScreenXYWithIndex(ctx, cca.DocumentFixModeCorner, i)
			if err != nil {
				return err
			}
			pt.X += dotElSize.Width / 2
			pt.Y += dotElSize.Height / 2
			corners[i] = docCorner{
				float64(pt.X-imageElScreenXY.X) / float64(imageElSize.Width),
				float64(pt.Y-imageElScreenXY.Y) / float64(imageElSize.Height)}
		}

		initialDocArea := &docArea{corners}
		if err := doc1Area.checkSimilar(initialDocArea); err != nil {
			return errors.Wrap(err, "Mismatch document corner coordinate at initial scan")
		}

		// Drag corners to longer doc on right side. Drag must be done in
		// clockwise order to prevent hitting any checking convex constraint.
		for i := len(doc2Area.corners) - 1; i >= 0; i-- {
			toScreenPt := func(corn docCorner) coords.Point {
				return coords.NewPoint(
					int(corn.x*float64(imageElSize.Width))+imageElScreenXY.X,
					int(corn.y*float64(imageElSize.Height))+imageElScreenXY.Y,
				)
			}
			if err := mouse.Drag(
				tconn, toScreenPt(initialDocArea.corners[i]), toScreenPt(doc2Area.corners[i]),
				300*time.Millisecond)(ctx); err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to drag corner"))
			}
		}

		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second})
	if err != nil {
		return err
	}

	if err := app.Click(ctx, cca.DocumentDoneFixButton); err != nil {
		return errors.Wrap(err, "failed to click the done button")
	}

	if err := app.WaitForVisibleState(ctx, cca.DocumentPreviewModeImage, true); err != nil {
		return errors.Wrap(err, "failed to wait for preview mode image to show up")
	}

	imageElSize, err = app.Size(ctx, cca.DocumentPreviewModeImage)
	if err != nil {
		return err
	}
	if imageElSize.Width >= imageElSize.Height {
		return errors.Errorf("should crop the longer document after fix crop area, got document width: %v, height: %v", imageElSize.Width, imageElSize.Height)
	}

	return nil
}

// testPDFOCR verifies if the saved PDF has `expectedText`.
func testPDFOCR(ctx context.Context, app *cca.App, cr *chrome.Chrome, bt browser.Type, expectedText, outDirForUITreeDump string) (retErr error) {
	if err := clickShutterAndWaitFor(ctx, app, cca.DocumentReview); err != nil {
		return errors.Wrap(err, "failed to wait for review UI to show")
	}

	start := time.Now()

	if err := app.Click(ctx, cca.DocumentSaveAsPdfButton); err != nil {
		return errors.Wrap(err, "failed to click save as PDF button")
	}

	file, err := waitForFileSaved(ctx, app, cca.DocumentPDFPattern, start)
	if err != nil {
		return errors.Wrap(err, "failed to wait for file to be saved")
	}

	path, err := app.FilePathInSavedDir(ctx, file.Name())
	if err != nil {
		return errors.Wrap(err, "failed to get full file path")
	}
	// Convert the file path to a URL that can be opened in the browser.
	// The original file path: /home/user/<id>/MyFiles/Camera/xxx.pdf
	// The URL we want: file:///home/chronos/u-<id>/MyFiles/Camera/xxx.pdf
	parts := strings.Split(path, "/")
	parts[2] = "chronos"
	parts[3] = "u-" + parts[3]
	url := "file://" + strings.Join(parts, "/")
	testing.ContextLog(ctx, "File path: ", path)
	testing.ContextLog(ctx, "URL: ", url)

	br, brCleanUp, err := browserfixt.Connect(ctx, cr, bt)
	if err != nil {
		return errors.Wrap(err, "failed to set up browser")
	}
	defer brCleanUp(ctx)

	// Open `url` in the browser.
	conn, err := br.NewConn(ctx, url)
	if err != nil {
		return errors.Wrap(err, "failed to open url")
	}
	defer br.CloseTarget(ctx, conn.TargetID)
	defer conn.Close()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Test API connection")
	}

	// For debugging when the test fails to find the expected text on screen.
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, outDirForUITreeDump, func() bool { return retErr != nil }, cr, "ui_tree")

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Role(role.StaticText).NameStartingWith(expectedText))(ctx); err != nil {
		return errors.Wrapf(err, "failed to find %q on screen", expectedText)
	}

	return nil
}

// clickShutterAndWaitFor clicks shutter button and waits specified UI for 10 seconds
func clickShutterAndWaitFor(ctx context.Context, app *cca.App, ui cca.UIComponentName) error {
	if err := app.ClickShutter(ctx); err != nil {
		return errors.Wrap(err, "failed to click the shutter button")
	}

	if err := app.WaitForVisibleStateFor(ctx, ui, true, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for UI")
	}

	return nil
}

func waitForFileSaved(ctx context.Context, app *cca.App, pat *regexp.Regexp, start time.Time) (fs.FileInfo, error) {
	var file fs.FileInfo
	dir, err := app.SavedDir(ctx)
	if err != nil {
		return file, errors.Wrap(err, "failed to get CCA default saved path")
	}

	file, err = app.WaitForFileSavedFor(ctx, dir, pat, start, 10*time.Second)
	if err != nil {
		return file, errors.Wrap(err, "failed to wait for the file")
	}

	return file, nil
}
