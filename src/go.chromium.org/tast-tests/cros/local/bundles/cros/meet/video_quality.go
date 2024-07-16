// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"context"
	"fmt"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const originalHeaderLen = 10
const frameLengthFactor = 1.5
const floatBitSize = 32
const maxAvgColorDist = 25

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoQuality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that checks DUT video quality",
		Contacts: []string{
			"core-devices@google.com",
			"torikauffman@google.com", // Test author
			"egwuekwe@google.com",     // Test author
		},

		BugComponent: "b:543707", // Communications > Video (Meet) > Platforms > Rooms > Core Devices (OS & Hardware)
		Attr:         []string{"group:meet", "group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "meet_device"},
		Timeout:      chrome.LoginTimeout + 45*time.Second,
		Data:         []string{"esfr_iso_test.y4m"},
		VarDeps: []string{
			"meet.VideoQuality.user",     // GAIA username.
			"meet.VideoQuality.password", // GAIA password.
		},
	})
}

func VideoQuality(ctx context.Context, s *testing.State) {
	// Set default tags for a fresh ash-chrome instance (cr).
	tags := []string{
		"login_display_host*=4",
		"oobe_ui=4",
	}

	cfmUser := s.RequiredVar("meet.VideoQuality.user")
	cfmPassword := s.RequiredVar("meet.VideoQuality.password")

	videoPath := s.DataPath("esfr_iso_test.y4m")

	opts := append([]chrome.Option{
		chrome.ExtraArgs("--enable-logging",
			"--use-fake-ui-for-media-stream",
			"--use-fake-device-for-media-stream",
			"--use-file-for-fake-video-capture="+videoPath,
			"--vmodule="+strings.Join(tags, ","))},
		chrome.NoLogin(),
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: cfmUser, Pass: cfmPassword}))

	opts = append(opts, chrome.RemoveNotification(false),
		chrome.DontWaitForCryptohome())
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	conn, err := cr.WaitForCFMConnection(ctx)
	if err != nil {
		s.Fatal("Failed to load CFM OOBE connection: ", err)
	}
	defer conn.Close()

	// Wait for the page to load with the video data
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var videoFound bool
		if err := conn.Eval(ctx, "document.getElementsByTagName('video').length != 0", &videoFound); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get video count"))
		}
		if !videoFound {
			return errors.New("No videos found on loading screen")
		}
		var videoReady bool
		if err := conn.Eval(ctx, "document.getElementsByTagName('video')[0].readyState > 2", &videoReady); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get video ready state"))
		}
		if videoReady {
			return nil
		}
		return errors.Errorf("Video not yet loaded; invalid video readyState found")

	}, &testing.PollOptions{
		Timeout:  15 * time.Second,
		Interval: 1 * time.Second,
	}); err != nil {
		s.Fatal("Failed to wait for video preview: ", err)
	}

	// Get video data from reference and CfM
	s.Log("Getting original video frames")
	height, width, originalFrame := getOriginalFrames(videoPath, s)
	s.Log("Getting device frames")
	outputFrame := getOutputFrames(ctx, s, conn, height, width)

	s.Log("Computing average pixel color distance")
	avgDist := getAvgColorDifference(outputFrame, originalFrame, height, width, s)
	s.Log("Found average distance of: ", avgDist)

	if avgDist > maxAvgColorDist {
		s.Fatalf("Minimum video quality not found; expected average pixel distance of less than %d; found %f", maxAvgColorDist, avgDist)
	}
}

// displayOriginalFrame print a canvas to the provided connection given frame data
func displayOriginalFrame(ctx context.Context, originalFrames []byte, height, width int, conn *chrome.Conn, s *testing.State) {
	rgbArr := getColorData(originalFrames, height, width)
	rgbStr := strings.Join(rgbArr, ", ")

	exp := fmt.Sprintf(`
		const arr = [%s];
		const pixels = new Uint8ClampedArray(arr);
		const printCanvas = document.createElement("canvas");
		const printCtx = print_canvas.getContext('2d');
		printCtx.canvas.width  = %d;
		printCtx.canvas.height = %d;
		var imageData = printCtx.createImageData(printCtx.canvas.width, printCtx.canvas.height);
		for(var i = 0; i < imageData.data.length; i++) {
			imageData.data[i] = pixels[i];
		}
		printCtx.putImageData(imageData, 0, 0);
		document.body.appendChild(printCanvas);`,
		rgbStr, width, height)

	if err := conn.Eval(ctx, exp, nil); err != nil {
		s.Fatal("Failed to read cfm frames: ", err)
	}
}

// getColorData returns a RGBA color data array given a frame from a y4m file
func getColorData(y4m []byte, height, width int) []string {
	numPixels := height * width
	colorData := make([]string, numPixels*4)

	for i := 0; i < numPixels; i++ {
		r, g, b := convertPixelColor(y4m, height, width, i)
		colorData[i*4] = fmt.Sprint(r)
		colorData[i*4+1] = fmt.Sprint(g)
		colorData[i*4+2] = fmt.Sprint(b)
		colorData[i*4+3] = "255"
	}
	return colorData
}

// getOutputFrames returns frames from the video stream in the cfm
func getOutputFrames(ctx context.Context, s *testing.State, conn *chrome.Conn, height, width int) []byte {
	exp := fmt.Sprintf(`
		const video = document.getElementsByTagName("video")[0];
		const canvas = document.createElement("canvas");
		const context = canvas.getContext('2d');
		context.canvas.width = %d;
		context.canvas.height = %d;
		video.currentTime = 0;
		context.drawImage(video, 0, 0, context.canvas.width, context.canvas.height);
		Array.from(context.getImageData(0, 0, context.canvas.width, context.canvas.height).data);`,
		width, height)

	var outputFrames []byte
	if err := conn.Eval(ctx, exp, &outputFrames); err != nil {
		s.Fatal("Failed to read CfM frames: ", err)
	}
	return outputFrames
}

// getOriginalFrames returns the sample y4m frames from the reference video
func getOriginalFrames(videoPath string, s *testing.State) (int, int, []byte) {
	originalBytes, err := os.ReadFile(videoPath)
	if err != nil {
		s.Fatal("Failed to read y4m file: ", err)
	}

	originalBytes = originalBytes[originalHeaderLen:]
	originalBytesStr := string(originalBytes)

	// Extract height and width of original image
	heightIdx := strings.Index(originalBytesStr, "H") + 1
	heightIdxEnd := strings.Index(originalBytesStr[heightIdx:], " ") + heightIdx
	height, err := strconv.ParseFloat(originalBytesStr[heightIdx:heightIdxEnd], floatBitSize)
	if err != nil {
		s.Fatal("Failed to parse sample video height: ", err)
	}

	widthIdx := strings.Index(originalBytesStr, "W") + 1
	widthIdxEnd := strings.Index(originalBytesStr[widthIdx:], " ") + widthIdx
	width, err := strconv.ParseFloat(originalBytesStr[widthIdx:widthIdxEnd], floatBitSize)
	if err != nil {
		s.Fatal("Failed to parse sample video width: ", err)
	}

	// Based on C420mpeg2 pixel ratios
	frameLength := int(width * height * frameLengthFactor)

	// Get beginning of first frame to end of next
	frameIdx := strings.Index(originalBytesStr, "FRAME") + 1
	frameStartChar := strings.IndexByte(originalBytesStr[frameIdx:], 0x0a) + frameIdx + 1
	frameIdxEnd := frameLength + frameStartChar

	frame := originalBytes[frameStartChar:frameIdxEnd]

	return int(height), int(width), frame
}

// convertPixelColor returns RGB values from the pixel at index i in the test frame
func convertPixelColor(frame []byte, height, width, i int) (byte, byte, byte) {
	y := frame[i]

	// Compute index of the cb and cr channels within the frame array for this pixel
	rowNum := (i / width) / 2
	idxStart := width*height + rowNum*(width/2) + (i%width)/2

	cb := frame[idxStart]
	cr := frame[idxStart+width*height/4]

	// Convert colors to RGB to compare against CfM frame
	return color.YCbCrToRGB(y, cb, cr)
}

// getColorDifference returns the Euclidean distance between two points
func getColorDifference(r1, g1, b1, r2, g2, b2 byte) float64 {
	dist := math.Pow(float64(r2)-float64(r1), 2) + math.Pow(float64(g2)-float64(g1), 2) + math.Pow(float64(b2)-float64(b1), 2)
	return math.Sqrt(dist)
}

// getAvgColorDifference returns the average distance per-pixel across two RGBA frames
func getAvgColorDifference(cfm, y4m []byte, height, width int, s *testing.State) float64 {
	var totDist float64
	totDist = 0
	numPixels := height * width

	for i := 0; i < numPixels; i++ {
		cfmStart := i * 4
		outputPixels := cfm[cfmStart : cfmStart+4]
		originalR, originalG, originalB := convertPixelColor(y4m, height, width, i)
		totDist += getColorDifference(outputPixels[0], outputPixels[1], outputPixels[2], originalR, originalG, originalB)
	}

	return totDist / float64(numPixels)
}
