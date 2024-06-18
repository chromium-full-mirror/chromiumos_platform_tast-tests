// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gallerycuj provides gallery instance for cuj tests on ChromeOS.
package gallerycuj

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	pngZipFile = "gallery_image.zip"
	pdfZipFile = "gallery_pdf.zip"
	videoFile1 = "gallery_video1.mp4"
	videoFile2 = "gallery_video2.mp4"
	videoFile3 = "gallery_video3.mp4"
	audioFile1 = "gallery_five_minute_audio.mp3"
	audioFile2 = "gallery_audio.mp3"
	audioFile3 = "gallery_audio.wav"
)

// GalleryFiles are the data files required by GalleryCUJ.
var GalleryFiles = []string{
	pngZipFile,
	pdfZipFile,
	videoFile1,
	videoFile2,
	videoFile3,
	audioFile1,
	audioFile2,
	audioFile3,
}

var videoFiles = []string{
	videoFile1,
	videoFile2,
	videoFile3,
}

var audioFiles = []string{
	audioFile1,
	audioFile2,
	audioFile3,
}

// patternPoints is the coordinates of drawing star pattern.
var patternPoints = []coords.Point{
	coords.Point{X: 1, Y: -8},
	coords.Point{X: 19, Y: -7},
	coords.Point{X: 25, Y: -25},
	coords.Point{X: 30, Y: -8},
	coords.Point{X: 48, Y: -7},
	coords.Point{X: 33, Y: 3},
	coords.Point{X: 39, Y: 18},
	coords.Point{X: 25, Y: 9},
	coords.Point{X: 11, Y: 18},
	coords.Point{X: 16, Y: 3},
	coords.Point{X: 1, Y: -8},
}

// galleryCUJTestResource represents test resources for testing ui.GalleryCUJ.
type galleryCUJTestResource struct {
	cr            *chrome.Chrome
	tconn         *chrome.TestConn
	filesApp      *filesapp.FilesApp
	kb            *input.KeyboardEventWriter
	recorder      *cujrecorder.Recorder
	downloadsPath string
	outDir        string
}

const (
	imageDir = "image"
	pdfDir   = "pdf"
	videoDir = "video"
	audioDir = "audio"
)

// Run runs the setup, core part of the GalleryCUJ test, and cleanup.
func Run(ctx context.Context, cr *chrome.Chrome, outDir string, dataPath func(string) string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return errors.Wrap(err, "failed to capture device snapshot")
	}

	// Mute the device. We don’t track audio performance anyways, so we don’t
	// want the video or audio file disturbing anyone, especially during development.
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to CRAS")
	}
	if err := cras.SetInputMute(ctx, true); err != nil {
		return errors.Wrap(err, "cannot mute input")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create a keyboard")
	}
	defer kb.Close(cleanupCtx)

	// Get the net export log file.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}

	pdfPath := filepath.Join(downloadsPath, pdfDir)
	videoPath := filepath.Join(downloadsPath, videoDir)
	audioPath := filepath.Join(downloadsPath, audioDir)

	for _, dir := range []string{pdfPath, videoPath, audioPath} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}
	defer removeAllFilesInDirectory(downloadsPath)

	imageFileLocation := filepath.Join(downloadsPath, pngZipFile)
	if err := fsutil.CopyFile(dataPath(pngZipFile), imageFileLocation); err != nil {
		return errors.Wrapf(err, "failed to copy the test file %q to Downloads", pngZipFile)
	}
	if err := unzipTestFiles(ctx, imageFileLocation, downloadsPath); err != nil {
		return errors.Wrapf(err, "failed to unzip %v from %v", imageFileLocation, downloadsPath)
	}

	pdfFileLocation := filepath.Join(pdfPath, pdfZipFile)
	if err := fsutil.CopyFile(dataPath(pdfZipFile), pdfFileLocation); err != nil {
		return errors.Wrapf(err, "failed to copy the test file %q to Downloads", pdfZipFile)
	}
	if err := unzipTestFiles(ctx, pdfFileLocation, pdfPath); err != nil {
		return errors.Wrapf(err, "failed to unzip %v from %v", pdfFileLocation, pdfPath)
	}

	for _, fileName := range videoFiles {
		videoFileLocation := filepath.Join(videoPath, fileName)
		if err := fsutil.CopyFile(dataPath(fileName), videoFileLocation); err != nil {
			return errors.Wrapf(err, "failed to copy the test file %q to Downloads", fileName)
		}
	}

	for _, fileName := range audioFiles {
		audioFileLocation := filepath.Join(audioPath, fileName)
		if err := fsutil.CopyFile(dataPath(fileName), audioFileLocation); err != nil {
			return errors.Wrapf(err, "failed to copy the test file %q to Downloads", fileName)
		}
	}

	res := &galleryCUJTestResource{
		cr:            cr,
		tconn:         tconn,
		kb:            kb,
		downloadsPath: downloadsPath,
		outDir:        outDir,
	}

	// Shorten context to allow for cleanup. Reserve one minute in case of test.
	closeCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to create a recorder")
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		testing.ContextLog(ctx, "Failed to add screenshot recorder: ", err)
	}
	res.recorder = recorder

	if err := recorder.AddCommonMetrics(tconn, tconn); err != nil {
		return errors.Wrap(err, "failed to add common metrics to the recorder")
	}

	mediaMetrics := []cujrecorder.MetricConfig{
		cujrecorder.NewCustomMetricConfig("MediaApp.Image.Crop", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.Image.SaveTime", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.LoadTime.Translation", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.LoadTime.ImageHandler.FileLoad", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.LoadTime.PdfHandler.FileLoad", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.LoadTime.VideoHandler.FileLoad", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.LoadTime.AudioHandler.FileLoad", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.PDF.FormFillingFontPreload", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.Vitals.FCP", "ms", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("MediaApp.Vitals.LCP", "ms", perf.SmallerIsBetter),
	}
	if err := recorder.AddCollectedMetrics(tconn, browser.TypeAsh, mediaMetrics...); err != nil {
		return errors.Wrap(err, "failed to add media metrics to the recorder")
	}

	startTime := time.Now()
	if err := recorder.Run(ctx, func(ctx context.Context) (retErr error) {
		filesApp, err := filesapp.Launch(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to launch the Files app")
		}
		defer filesApp.Close(cleanupCtx)
		defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, outDir, func() bool { return retErr != nil }, tconn, "ui_dump_file")

		res.filesApp = filesApp
		if err := testImages(ctx, res); err != nil {
			return errors.Wrap(err, "failed to test Images")
		}
		if err := testPDF(ctx, res); err != nil {
			return errors.Wrap(err, "failed to test PDF")
		}
		if err := testVideoPlayback(ctx, res); err != nil {
			return errors.Wrap(err, "failed to test Video playback")
		}
		if err := testAudioPlayback(ctx, res); err != nil {
			return errors.Wrap(err, "failed to test Audio playback")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to conduct the recorder task")
	}
	testing.ContextLogf(ctx, "Recorder took %v to execute the test", time.Now().Sub(startTime))

	if err := recorder.Record(ctx, pv); err != nil {
		return errors.Wrap(err, "failed to report")
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files")
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		return errors.Wrap(err, "failed to save histogram raw data")
	}
	if err := pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to store values")
	}

	return nil
}

// testImages tests images as the following steps.
// [Setup]
//  1. Upload a zip file of 3 large png files to the Downloads folder.
//  2. Unarchive the zip file.
//  3. Split screen Files (left) and Gallery (right).
//  4. Select all files, and drag them to the Gallery app.
//  5. Maximize the Gallery window. (files should still be in the background)
//
// [Test]
//  1. Toggle the info button –[ test smoothness of open / close animation],
//     5 seconds open and then close.
//  2. Click on 'Crop & rotate' and click 'Rotate clockwise' twice. (picture
//     should be upside down)
//  3. Click '16:9' to crop the image.
//  4. Click 'Done' to save the changes.
//  5. Click Lighting filters.
//  6. Click on the slider for Exposure.
//  7. Use the left arrow key [key press latency] and press it 10 times, with
//     a 500 millisecond pause in between each button press.
//  8. Click 'Save'.
//  9. Click 'Draw'.
//  10. Drag mouse around the file to draw.
//  11. Click 'Save'.
//  12. Cycle through all of the images 2-3 times.
//  13. Close the Gallery window.
func testImages(ctx context.Context, res *galleryCUJTestResource) (retErr error) {
	cr := res.cr
	tconn := res.tconn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	imagePath := filepath.Join(res.downloadsPath, imageDir)
	pngFiles, err := getFileNames(ctx, imagePath, "png")
	if err != nil {
		return errors.Wrap(err, "failed to get png file names")
	}
	if err := setWindowState(tconn, apps.Files.Name, ash.WindowStatePrimarySnapped)(ctx); err != nil {
		return errors.Wrap(err, "failed to set Files App left-snapped")
	}
	gallery, err := galleryapp.Launch(ctx, cr, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch gallery app")
	}
	defer gallery.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, cr, "ui_dump_png")

	if err := uiauto.NamedCombine("setup png files",
		setWindowState(tconn, apps.Gallery.Name, ash.WindowStateSecondarySnapped),
		res.filesApp.OpenDownloads(),
		res.filesApp.OpenFile(imageDir),
		dragAllFilesToGallery(res.kb, tconn),
		gallery.WaitImageOpened(),
		gallery.MaximizeWindow(),
		gallery.WaitForGalleryQuiescence(cr),
	)(ctx); err != nil {
		return err
	}

	var currentImageName string
	for i := range pngFiles {
		if i != 0 {
			if err := gallery.NavigateNext()(ctx); err != nil {
				return err
			}
		}
		currentImageName, err := gallery.WaitNameChanged(ctx, currentImageName)
		if err != nil {
			return err
		}
		if err := uiauto.NamedCombine("test png file: "+currentImageName,
			gallery.OpenInfo(),
			uiauto.Sleep(5*time.Second),
			gallery.CloseInfo(),
			gallery.SetCropAndRotate(galleryapp.RotateClockwise, 2),
			gallery.SetRatio(galleryapp.Ratio16by9),
			uiauto.Sleep(time.Second),
			gallery.Done(),
			gallery.SetLightingFilters(res.kb, galleryapp.SliderExposure, "Left", 10),
			gallery.Save(),
			waitForAndCloseDownloadCompleteNotification(tconn),
			gallery.DrawOnImage(patternPoints),
			gallery.Save(),
			waitForAndCloseDownloadCompleteNotification(tconn),
		)(ctx); err != nil {
			return err
		}
	}
	// Take a screenshot to see the state of testing images.
	res.recorder.CustomScreenshot(ctx)

	return nil
}

// testPDF tests PDF as the following steps.
// [Setup]
//  1. Split screen files (left) and Gallery (right)
//  2. Add a zip file with a single large PDF file to the Downloads folder,
//     and also the rest of the PDFs.
//     a. Look at “File Selection” for guidelines on the PDF files.
//  3. Unzip the zip file.
//  4. Drag PDF to Gallery app.
//  5. Maximize the Gallery window. (files should still be in the background)
//
// [Test]
//  1. [Mouse Wheel Latency] scroll the file for 5 seconds.
//  2. [Mouse Press/Release Latency] Press the Add text button.
//  3. [Key Press Latency] Double Click on the center of the screen to start
//     typing on the PDF file, and type 'The quick brown fox jumps over the
//     lazy dog in the afternoon on Saturday!'.
//  4. Create a signature.
//     a. Click on 'Sign'.
//     b. Click on 'Draw signature'.
//     c. Draw some random signature in the draw area.
//     d. Click 'Place Signature'.
//     e. Drag signature around and verify that it generates mouse move latency.
//  5. Click 'Draw'.
//  6. Drag mouse around the file to draw.
//  7. Click 'Save'.
//  8. Close the Gallery window.
//  9. Repeat 1-8 with the other PDFs (should be 3 more)
func testPDF(ctx context.Context, res *galleryCUJTestResource) error {
	const (
		testText          = "The quick brown fox jumps over the lazy dog in the afternoon on Saturday!"
		mouseWheelLatency = "EventLatency.MouseWheel.TotalLatency"
		mouseMoveLatency  = "EventLatency.MouseMoved.TotalLatency"
	)
	cr := res.cr
	tconn := res.tconn
	signPoints := []coords.Point{
		coords.Point{X: -150, Y: -100},
		coords.Point{X: 0, Y: -50},
		coords.Point{X: 150, Y: -100},
		coords.Point{X: 20, Y: 100},
		coords.Point{X: -30, Y: 80},
	}

	mw, err := input.Mouse(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a mouse")
	}
	defer mw.Close(ctx)

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the primary display info")
	}
	center := info.Bounds.CenterPoint()

	pdfPath := filepath.Join(res.downloadsPath, pdfDir)
	pdfFiles, err := getFileNames(ctx, pdfPath, pdfDir)
	if err != nil {
		return errors.Wrap(err, "failed to get pdf file names")
	}

	testPDFFile := func(ctx context.Context, pdfFile string) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()
		if err := setWindowState(tconn, apps.Files.Name, ash.WindowStatePrimarySnapped)(ctx); err != nil {
			return errors.Wrap(err, "failed to set Files App left-snapped")
		}

		gallery, err := galleryapp.Launch(ctx, cr, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to launch gallery app")
		}
		defer gallery.Close(cleanupCtx)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, cr, "ui_dump_pdf")

		if err := uiauto.NamedCombine("setup PDF file: "+pdfFile,
			setWindowState(tconn, apps.Gallery.Name, ash.WindowStateSecondarySnapped),
			res.filesApp.OpenDownloads(),
			res.filesApp.OpenFile(pdfDir),
			dragFilesToGallery(res.kb, tconn, res.filesApp, []string{pdfFile}),
			gallery.DismissPDFDialog(),
			gallery.MaximizeWindow(),
			gallery.WaitPDFOpened(),
			gallery.WaitForGalleryQuiescence(cr),
		)(ctx); err != nil {
			return err
		}

		return uiauto.NamedCombine("test PDF file: "+pdfFile,
			gallery.ClickOnPDFCanvas(),
			runAndWaitHistogram(tconn, scrollByMouse(mw, 5*time.Second), mouseWheelLatency),
			gallery.AddTextToPDF(res.kb, center, testText),
			gallery.Sign(),
			gallery.CreateSignature(signPoints),
			gallery.PlaceSignature(),
			runAndWaitHistogram(tconn, runDragMouseCycle(tconn, info), mouseMoveLatency),
			gallery.DrawOnPDF(patternPoints),
			gallery.Save(),
			waitForAndCloseDownloadCompleteNotification(tconn),
		)(ctx)
	}
	for _, pdfFile := range pdfFiles {
		if err := testPDFFile(ctx, pdfFile); err != nil {
			return err
		}
	}
	// Take a screenshot to see the state of testing PDF.
	res.recorder.CustomScreenshot(ctx)

	return nil
}

// testVideoPlayback tests Video playback as the following steps.
// [Setup]
//  1. Split screen files (left) and Gallery (right).
//  2. Add 3 large video files to the Downloads folder.
//  3. Drag all video files to the Gallery app.
//  4. Maximize the Gallery window. (files should still be in the background)
//
// [Test]
//  1. Press play on the video for 15 seconds.
//  2. Maximize the video, and press play for 15 seconds.
//  3. Toggle 'Picture in picture', and play for 15 seconds.
//  4. Exit 'Picture in picture' and exit full screen.
//  5. Toggle the More Info button. (make sure the panel fully expands before
//     clicking again)
//  6. Go to the next video (cycling through similar to the PNG section), and
//     repeat steps 1-5.
//  7. Close Gallery window.
func testVideoPlayback(ctx context.Context, res *galleryCUJTestResource) (retErr error) {
	const playTime = 15 * time.Second
	cr := res.cr
	tconn := res.tconn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	gallery, err := galleryapp.Launch(ctx, cr, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch gallery app")
	}
	defer gallery.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, cr, "ui_dump_video")

	if err := uiauto.NamedCombine("setup video files",
		setWindowState(tconn, apps.Gallery.Name, ash.WindowStateSecondarySnapped),
		res.filesApp.OpenDownloads(),
		res.filesApp.OpenFile(videoDir),
		dragFilesToGallery(res.kb, tconn, res.filesApp, videoFiles),
		gallery.MaximizeWindow(),
		gallery.WaitForGalleryQuiescence(cr),
	)(ctx); err != nil {
		return err
	}

	var currentVideoName string
	for i := range videoFiles {
		if i != 0 {
			if err := gallery.NavigateNext()(ctx); err != nil {
				return err
			}
		}
		currentVideoName, err := gallery.WaitNameChanged(ctx, currentVideoName)
		if err != nil {
			return err
		}
		if err := uiauto.NamedCombine("test video: "+currentVideoName,
			gallery.WaitUntilPaused,
			gallery.TogglePlayPause(true),
			uiauto.NamedAction("sleep for 15 seconds", uiauto.Sleep(playTime)),
			gallery.ToggleFullscreen(true),
			uiauto.NamedAction("sleep for 15 seconds", uiauto.Sleep(playTime)),
			gallery.SetPIP(true),
			uiauto.NamedAction("sleep for 15 seconds", uiauto.Sleep(playTime)),
			gallery.SetPIP(false),
			gallery.ToggleFullscreen(false),
			gallery.OpenInfo(),
			uiauto.NamedAction("sleep for 5 seconds", uiauto.Sleep(5*time.Second)),
			gallery.CloseInfo(),
		)(ctx); err != nil {
			return err
		}
	}
	// Take a screenshot to see the state of testing video playback.
	res.recorder.CustomScreenshot(ctx)

	return nil
}

// testAudioPlayback tests Audio playback as the following steps.
// [Setup]
//  1. Split screen files (left) and Gallery (right).
//  2. Add 3 large audio files to the Downloads folder.
//  3. Drag all audio files to the Gallery.
//  4. Maximize the Gallery window. (files should still be in the background)
//
// [Test]
//  1. Play for 10 seconds.
//  2. Press Skip forward 10 seconds, sleep 10 seconds, and then press Step
//     backwards 10 seconds, and sleep 10 seconds.
//  3. Go back to the start of the track, and change playback speed to 2x.
//  4. Play for 10 seconds
//  5. Repeat for 2 other tracks that were opened.
func testAudioPlayback(ctx context.Context, res *galleryCUJTestResource) (retErr error) {
	cr := res.cr
	tconn := res.tconn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	gallery, err := galleryapp.Launch(ctx, cr, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch gallery app")
	}
	defer gallery.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, cr, "ui_dump_audio")

	if err := uiauto.NamedCombine("setup audio files",
		setWindowState(tconn, apps.Gallery.Name, ash.WindowStateSecondarySnapped),
		res.filesApp.OpenDownloads(),
		res.filesApp.OpenFile(audioDir),
		dragFilesToGallery(res.kb, tconn, res.filesApp, audioFiles),
		gallery.MaximizeWindow(),
		gallery.WaitForGalleryQuiescence(cr),
	)(ctx); err != nil {
		return err
	}

	var currentAudioName string
	for i := range audioFiles {
		if i != 0 {
			if err := gallery.SkipNext()(ctx); err != nil {
				return err
			}
		}
		currentAudioName, err := gallery.WaitNameChanged(ctx, currentAudioName)
		if err != nil {
			return err
		}
		if err := uiauto.NamedCombine("test audio: "+currentAudioName,
			gallery.TogglePlayPause(true),
			uiauto.NamedAction("play audio for 10 seconds", uiauto.Sleep(10*time.Second)),
			gallery.StepForwardTenSeconds(),
			uiauto.NamedAction("sleep for 10 seconds", uiauto.Sleep(10*time.Second)),
			gallery.StepBackwardTenSeconds(),
			uiauto.NamedAction("sleep for 10 seconds", uiauto.Sleep(10*time.Second)),
			gallery.GoToTrackStart(),
			// Set playback speed to 2x.
			gallery.SetPlaybackSpeed(galleryapp.SpeedOption5),
			uiauto.NamedAction("sleep for 10 seconds", uiauto.Sleep(10*time.Second)),
		)(ctx); err != nil {
			return err
		}
	}
	// Take a screenshot to see the state of testing audio playback.
	res.recorder.CustomScreenshot(ctx)

	return nil
}
