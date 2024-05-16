// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

/*
 * Why this test?
 *
 * We need a validation that the internal display is functional. Best way to do that is to check if the screen is emitting a colored light that we can detect using the camera.
 * There are different ways to detect this change.
 * Test 1: Compare the file sizes of the black and colored images. If the colored image is outside the tolerance of the average black image size, that means the screen is emitting light.
 * Test 2: Compare the ratio of the colors in the images. If the colored image has a different ratio of colors than the black image, that means the screen is emitting light.
 */

func init() {
	testing.AddTest(&testing.Test{
		Func: InternalPanelColorCompare,
		Desc: "Checks that the internal panel is emitting a colored light that we can detect using the camera to validate that Chrome appears on the screen",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		BugComponent: "b:188154", // ChromeOS > Platform > Graphics > Display
		// We want to test this on multiple models even within the same selection pool, so add more tests to hit more devices per day.
		Attr:         []string{"group:graphics", "graphics_nightly", "graphics_perbuild", "graphics_weekly"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "gpuWatchHangs",
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func InternalPanelColorCompare(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(ctx)

	colorNames := []string{"red", "green", "blue"}

	// Capture black and colored images to compare them.
	blackImgs := make([]string, len(colorNames))
	coloredImgs := make([]string, len(colorNames))
	for idx, color := range colorNames {
		blackImg, coloredImg, err := getBlackAndColoredImages(ctx, cr, idx, color, s.OutDir())
		if err != nil {
			s.Fatal("Failed to get black and colored images: ", err)
		}
		blackImgs[idx] = blackImg
		coloredImgs[idx] = coloredImg
	}

	// Create a shuffled copy of the black images to test for false positives.
	shuffledBlackImgs := make([]string, len(blackImgs))
	copy(shuffledBlackImgs, blackImgs)
	rand.Shuffle(len(shuffledBlackImgs), func(i, j int) {
		shuffledBlackImgs[i], shuffledBlackImgs[j] = shuffledBlackImgs[j], shuffledBlackImgs[i]
	})

	// One way to compare images is to compare the file sizes. If the colored image is outside the
	// tolerance of the average black image size, that means the screen is emitting light.
	// Before we test out with colored images, let's check for false positives by comparing the black
	// images to themselves.
	success, err := testImgsWithFileSizes(ctx, blackImgs, shuffledBlackImgs)
	if err != nil {
		s.Fatal("Failed to compare B-B with sizes: ", err)
	}
	if success {
		s.Fatal("False positive detected on Image Size Test")
	}

	success, err = testImgsWithFileSizes(ctx, blackImgs, coloredImgs)
	if err != nil {
		s.Fatal("Failed to compare B-C with sizes: ", err)
	}
	if success {
		return
	}

	// If we fail to compare with the file sizes, compare the ratio of the colors in the images. If
	// the colored image has a different ratio of colors than the black image, that means the
	// screen is emitting light.
	if testImgsWithColorRatio(ctx, blackImgs, coloredImgs) {
		return
	}

	// If we fail to compare with the color ratios, compare the histograms of the images. If the colored
	// image has a different histogram than the black image, that means the screen is emitting light.
	// Test for false positives by comparing the black images to themselves first.
	if testImgsWithHistogramAnalysis(ctx, blackImgs, shuffledBlackImgs) {
		s.Fatal("False positive detected on Histogram Analysis Test")
	}

	if testImgsWithHistogramAnalysis(ctx, blackImgs, coloredImgs) {
		return
	}

	s.Fatalf("Failed to detect a change in the screen color on panel %s", getPanelName(ctx))
}

func getBlackAndColoredImages(ctx context.Context, cr *chrome.Chrome, idx int, color, outDir string) (string, string, error) {
	// Get a black image before the colored image
	if err := openColoredFullScreen(ctx, cr, "black"); err != nil {
		return "", "", errors.Wrap(err, "failed to open a black screen in fullscreen")
	}

	if err := setSystemBrightness(ctx, 0.0); err != nil {
		return "", "", errors.Wrap(err, "failed to turn the brightness down")
	}

	// Using .jpg because we want a lossy compression that puts similar colors together.
	blackImgPath := fmt.Sprintf("%s/black_shot_%d.jpg", outDir, idx)
	if err := takeCameraShotOnFullScreen(ctx, cr, blackImgPath); err != nil {
		return "", "", errors.Wrap(err, "failed to take a black camera shot")
	}

	// Get a colored image
	if err := openColoredFullScreen(ctx, cr, color); err != nil {
		return "", "", errors.Wrapf(err, "failed to open a %s screen in fullscreen", color)
	}

	if err := setSystemBrightness(ctx, 100.0); err != nil {
		return "", "", errors.Wrap(err, "failed to turn the brightness up")
	}

	coloredImgPath := fmt.Sprintf("%s/%s_shot_%d.jpg", outDir, color, idx)
	if err := takeCameraShotOnFullScreen(ctx, cr, coloredImgPath); err != nil {
		return "", "", errors.Wrapf(err, "failed to take a %s camera shot", color)
	}

	return blackImgPath, coloredImgPath, nil
}

func testImgsWithFileSizes(ctx context.Context, blackImgs, coloredImgs []string) (bool, error) {
	// Get the average file size of the black images and the standard deviation.
	var averageBlackSize int64 = 0
	var sumOfSquares int64 = 0
	for _, blackImg := range blackImgs {
		info, err := os.Stat(blackImg)
		if err != nil {
			return false, errors.Wrapf(err, "failed to stat %s", filepath.Base(blackImg))
		}

		averageBlackSize += info.Size()
		testing.ContextLogf(ctx, "Black image %s size: %d", filepath.Base(blackImg), info.Size())
		sumOfSquares += info.Size() * info.Size()
	}

	averageBlackSize /= int64(len(blackImgs))
	var variance float64 = float64(sumOfSquares)/float64(len(blackImgs)) - float64(averageBlackSize)*float64(averageBlackSize)
	const tolerance = 4
	standardDeviation := tolerance * math.Sqrt(variance)
	testing.ContextLogf(ctx, "Average black image size: %d with standard deviation: %f", averageBlackSize, standardDeviation)

	// Compare the average black image size with the colored image sizes.
	for _, coloredImg := range coloredImgs {
		info, err := os.Stat(coloredImg)
		if err != nil {
			return false, errors.Wrapf(err, "failed to stat %s", filepath.Base(coloredImg))
		}

		testing.ContextLogf(ctx, "Colored image %s size: %d", filepath.Base(coloredImg), info.Size())
		if info.Size() > averageBlackSize+int64(standardDeviation) || info.Size() < averageBlackSize-int64(standardDeviation) {
			return true, nil
		}
	}

	testing.ContextLog(ctx, "Failed to find a meaningful difference in file sizes")
	return false, nil
}

func testImgsWithColorRatio(ctx context.Context, blackImgs, coloredImgs []string) bool {
	res, err := checkForColorSignificance(ctx, "R", blackImgs, coloredImgs[0])
	if err != nil {
		testing.ContextLog(ctx, "testing Red with Color Ratio failed: ", err)
		return false
	}
	if res {
		testing.ContextLog(ctx, "Detected more Red")
		return true
	}

	res, err = checkForColorSignificance(ctx, "G", blackImgs, coloredImgs[1])
	if err != nil {
		testing.ContextLog(ctx, "testing Green with Color Ratio failed: ", err)
		return false
	}
	if res {
		testing.ContextLog(ctx, "Detected more Green")
		return true
	}

	res, err = checkForColorSignificance(ctx, "B", blackImgs, coloredImgs[2])
	if err != nil {
		testing.ContextLog(ctx, "testing Blue with Color Ratio failed: ", err)
		return false
	}
	if res {
		testing.ContextLog(ctx, "Detected more Blue")
		return true
	}

	return false
}

func testImgsWithHistogramAnalysis(ctx context.Context, blackImgs, coloredImgs []string) bool {
	// Test Red
	isRedMoreIntense, err := compareHistogramIntensity(ctx, "red", blackImgs[0], coloredImgs[0])
	if err != nil {
		testing.ContextLog(ctx, "testing Red with Histogram failed: ", err)
		return false
	}
	if isRedMoreIntense {
		testing.ContextLog(ctx, "Detected more Red")
		return true
	}

	// Test Green
	isGreenMoreIntense, err := compareHistogramIntensity(ctx, "green", blackImgs[1], coloredImgs[1])
	if err != nil {
		testing.ContextLog(ctx, "testing Green with Histogram failed: ", err)
		return false
	}
	if isGreenMoreIntense {
		testing.ContextLog(ctx, "Detected more Green")
		return true
	}

	// Test Blue
	isBlueMoreIntense, err := compareHistogramIntensity(ctx, "blue", blackImgs[2], coloredImgs[2])
	if err != nil {
		testing.ContextLog(ctx, "testing Blue with Histogram failed: ", err)
		return false
	}
	if isBlueMoreIntense {
		testing.ContextLog(ctx, "Detected more Blue")
		return true
	}

	testing.ContextLog(ctx, "Failed to find a meaningful difference in Histogram Analysis")
	return false
}

func openColoredFullScreen(ctx context.Context, cr *chrome.Chrome, htmlColor string) error {
	html := "<style>body { background-color: " + htmlColor + "; }</style>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		io.WriteString(w, html)
	}))
	defer server.Close()

	conn, err := cr.NewConn(ctx, server.URL)
	if err != nil {
		return errors.Wrap(err, "Creating renderer failed")
	}
	defer conn.Close()

	return nil
}

func toggleFullScreen(ctx context.Context, cr *chrome.Chrome) error {
	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the keyboard")
	}
	defer kb.Close(ctx)

	if err := kb.Accel(ctx, "f11"); err != nil {
		return errors.Wrap(err, "failed to type fullscreen hotkey")
	}

	return nil
}

func setSystemBrightness(ctx context.Context, percent float64) error {
	if err := testexec.CommandContext(ctx, "backlight_tool", fmt.Sprintf("--set_brightness_percent=%f", percent)).Run(); err != nil {
		return errors.Wrapf(err, "failed to set %f%% brightness", percent)
	}
	return nil
}

func takeCameraShotOnFullScreen(ctx context.Context, cr *chrome.Chrome, imgPath string) error {
	// Put the screen into fullscreen mode to emit the most light.
	if err := toggleFullScreen(ctx, cr); err != nil {
		return errors.Wrap(err, "failed to toggle the screen")
	}

	captureCmd := []string{"--user=arc-camera", "cros_camera_test",
		"--gtest_filter=Camera3StillCaptureTest/Camera3DumpSimpleStillCaptureTest.DumpCaptureResult/0",
		"--camera_facing=front",
		"--dump_still_capture_path=" + imgPath,
		"--connect_to_camera_service=false"}
	testing.ContextLog(ctx, "Running ", strings.Join(captureCmd, " "))
	if err := testexec.CommandContext(ctx, "sudo", captureCmd...).Run(); err != nil {
		return errors.Wrap(err, "failed to capture camera shot")
	}

	// Put the screen back to normal mode.
	if err := toggleFullScreen(ctx, cr); err != nil {
		return errors.Wrap(err, "failed to toggle the screen")
	}
	return nil
}

func checkForColorSignificance(ctx context.Context, color string, blackImgs []string, coloredImg string) (bool, error) {
	minColorness := 0.0
	for _, blackImg := range blackImgs {
		blackColorRatio, err := getColorToOtherColorRatio(ctx, color, blackImg)
		if err != nil {
			return false, errors.Wrapf(err, "failed to get color ratio for %s for image %s", color, blackImg)
		}
		if blackColorRatio > minColorness {
			minColorness = blackColorRatio
		}
	}

	minColorness *= 1.1 //  Add 10% tolerance to create an enough gap for a valid signal.

	colored, err := getColorToOtherColorRatio(ctx, color, coloredImg)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get color ratio for %s for image %s", color, coloredImg)
	}
	testing.ContextLogf(ctx, "%s: Minimum colorness: %f, Colored colorness: %f", color, minColorness, colored)

	return colored > minColorness, nil

}

func getColorToOtherColorRatio(ctx context.Context, colorAbbrev, imgPath string) (float64, error) {
	colorness, err := getSingleColorRatioToRgb(ctx, colorAbbrev, imgPath)
	if err != nil {
		return 0.0, err
	}

	// Get the average of the other 2 color channels to compare it against the tested color channel.
	averages := make([]float64, 0)

	if colorAbbrev != "R" {
		redness, err := getSingleColorRatioToRgb(ctx, "R", imgPath)
		if err != nil {
			return 0.0, err
		}
		averages = append(averages, redness)
	}

	if colorAbbrev != "G" {
		greenness, err := getSingleColorRatioToRgb(ctx, "G", imgPath)
		if err != nil {
			return 0.0, err
		}
		averages = append(averages, greenness)
	}

	if colorAbbrev != "B" {
		blueness, err := getSingleColorRatioToRgb(ctx, "B", imgPath)
		if err != nil {
			return 0.0, err
		}
		averages = append(averages, blueness)
	}

	var sum float64
	for _, value := range averages {
		sum += value
	}
	averageOthers := sum / float64(len(averages))

	ratio := colorness / averageOthers
	return ratio, nil
}

func getSingleColorRatioToRgb(ctx context.Context, colorAbbrev, imgPath string) (float64, error) {
	cmd := testexec.CommandContext(ctx, "convert", imgPath, "-colorspace", "RGB", "-channel", colorAbbrev, "-separate", "-format", "\"%[mean]\"", "info:")
	out, err := cmd.Output()
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get the color ratio of %s", colorAbbrev)
	}
	rawValue := strings.Trim(strings.TrimSpace(string(out)), "\"")
	color, err := strconv.ParseFloat(rawValue, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse the color ratio of %s with value %s", colorAbbrev, rawValue)
	}
	return color, nil
}

func compareHistogramIntensity(ctx context.Context, color, blackImg, coloredImg string) (bool, error) {
	threshold := 0.5

	blackHistogram, err := generateHistogramString(ctx, color, blackImg)
	if err != nil {
		return false, errors.Wrapf(err, "failed to generate histogram for %s", blackImg)
	}
	coloredHistogram, err := generateHistogramString(ctx, color, coloredImg)
	if err != nil {
		return false, errors.Wrapf(err, "failed to generate histogram for %s", coloredImg)
	}

	blackIntensity, blackCount, err := extractHistogramData(blackHistogram, color)
	if err != nil {
		return false, errors.Wrap(err, "failed to extract histogram data for black")
	}
	coloredIntensity, colorCount, err := extractHistogramData(coloredHistogram, color)
	if err != nil {
		return false, errors.Wrap(err, "failed to extract histogram data for colored")
	}

	blackMean := calculateWeightedMean(blackIntensity, blackCount)
	colorMean := calculateWeightedMean(coloredIntensity, colorCount)

	testing.ContextLogf(ctx, "Black Mean: %f, %s Mean: %f", blackMean, color, colorMean)
	return colorMean > blackMean+threshold, nil
}

func generateHistogramString(ctx context.Context, color, img string) (string, error) {
	cmd := testexec.CommandContext(ctx, "convert", img, "-format", "%c", "-channel", color, "histogram:")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func extractHistogramData(histogram, color string) (intensities, counts []float64, err error) {
	// Regular expression to match histogram lines like "  1234: (0,0,255)" to remove Exif data.
	// This represents "  1234: (R,G,B)" where 1234 is the count of pixels with the RGB value.
	// Each regex pattern captures the count and the intensity of the color.
	regexPatterns := map[string]string{
		"red":   `\s*(\d+):.*\((\d+),`,
		"green": `\s*(\d+):.*\(\d+,(\d+),`,
		"blue":  `\s*(\d+):.*\((?:\d+,){2}(\d+)\)`,
	}

	re := regexp.MustCompile(regexPatterns[color])
	lines := strings.Split(strings.TrimSpace(histogram), "\n")
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 { // Ensure we have 3 matches (full match + 2 captures)
			count, err := strconv.ParseFloat(matches[1] /*bitSize=*/, 64)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to parse count %s", matches[1])
			}
			intensity, err := strconv.ParseFloat(matches[2] /*bitSize=*/, 64)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to parse intensity %s", matches[2])
			}
			counts = append(counts, count)
			intensities = append(intensities, intensity)
		}
	}

	return intensities, counts, nil
}

func calculateWeightedMean(intensities, counts []float64) float64 {
	sum := 0.0
	totalWeight := 0.0
	for i := 0; i < len(intensities); i++ {
		sum += intensities[i] * counts[i]
		totalWeight += counts[i]
	}
	return sum / totalWeight
}

func getPanelName(ctx context.Context) string {
	connectors, err := graphics.ModetestConnectors(ctx)
	if err != nil {
		return "Failed to get the panel name" + err.Error()
	}

	for _, connector := range connectors {
		if strings.Contains(connector.Name, "eDP") {
			return fmt.Sprintf("%s %d", connector.Edid.ManufacturerName, connector.Edid.ModelNumber)
		}
	}

	return "Unknown"
}
