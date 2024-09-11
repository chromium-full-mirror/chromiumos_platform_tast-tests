// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type params struct {
	expectedID, resultID string
	referenceColors      []color.RGBA
}

var (
	params2d = params{
		expectedID: "expected2d",
		resultID:   "result2d",
		referenceColors: []color.RGBA{
			{255, 0, 0, 255},
			{0, 0, 255, 255},
			{0, 128, 0, 255},
			{255, 255, 255, 255},
		}}
	paramsWebgl = params{
		expectedID: "expectedWebGL",
		resultID:   "resultWebGL",
		referenceColors: []color.RGBA{
			{255, 0, 0, 255},
			{0, 0, 255, 255},
			{0, 255, 0, 255},
			{255, 255, 255, 255},
		},
	}
)

const delayToScreenshot = 7 * time.Second

func init() {
	testing.AddTest(&testing.Test{
		Func: CompositorTransferFromImageBitmapOrientation,
		Desc: "Verifies that transferFromImageBitmap is oriented correctly",
		Contacts: []string{
			"chromeos-gfx-compositor@google.com",
			"aswolfers@chromium.org",
		},
		BugComponent: "b:1021073",
		Attr:         []string{"group:graphics", "graphics_nightly"},
		Data:         []string{"transfer-from-image-bitmap.html"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Params: []testing.Param{{
			Name:    "2d",
			Val:     params2d,
			Fixture: "chromeGraphics",
		}, {
			Name:    "webgl",
			Val:     paramsWebgl,
			Fixture: "chromeGraphics",
		}},
	})
}

// CompositorTransferFromImageBitmapOrientation renders to offscreen canvases, transfers their
// contents to bitmap renderers, and screenshots the results. The screenshots are compared against
// screenshots of reference canvases to validate that the results are oriented correctly.
func CompositorTransferFromImageBitmapOrientation(ctx context.Context, s *testing.State) {
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	url := server.URL + "/transfer-from-image-bitmap.html"

	params := s.Param().(params)
	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(
		ctx, s.FixtValue().(chrome.HasChrome).Chrome(), browser.TypeAsh, url)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(ctx)
	defer conn.Close()

	tconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	if err := graphics.RotateDisplayToLandscapePrimary(ctx, tconn); err != nil {
		s.Fatal("Failed to set display to landscape-primary orientation: ", err)
	}

	if err := conn.Call(ctx, nil, "executeDraw"); err != nil {
		s.Fatal("Failed to execute drawing function: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize the keyboard writer: ", err)
	}
	defer kb.Close(ctx)

	// screenshotCanvas requests a fullscreen view of the specified element, takes a screenshot,
	// and returns the resulting image.
	screenshotCanvas := func(id string) (image.Image, error) {
		if err := conn.Call(ctx, nil, "setFullscreenEventListener", id); err != nil {
			return nil, errors.Wrapf(err,
				"failed to set fullscreen event listener on element %v", id)
		}
		if err := kb.TypeSequence(ctx, []string{"Esc", "f"}); err != nil {
			return nil, errors.Wrap(err,
				"failed to inject the 'Esc', 'F' key sequence")
		}
		// GoBigSleepLint: Wait for the fullscreen transition to complete and for the
		// escape message to disappear before taking the screenshot.
		if err := testing.Sleep(ctx, delayToScreenshot); err != nil {
			return nil, errors.Wrap(err, "failed to sleep prior to taking screenshot")
		}
		sshotPath := filepath.Join(s.OutDir(), fmt.Sprintf("%v.png", id))
		if err := screenshot.Capture(ctx, sshotPath); err != nil {
			return nil, errors.Wrapf(err, "failed to capture screenshot of element %v",
				id)
		}
		f, err := os.Open(sshotPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open %v", sshotPath)
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode %v", sshotPath)
		}
		return img, nil
	}

	expectedImg, err := screenshotCanvas(params.expectedID)
	if err != nil {
		s.Fatalf("Failed to screenshot canvas %v: %v", params.expectedID, err)
	}
	resultImg, err := screenshotCanvas(params.resultID)
	if err != nil {
		s.Fatalf("Failed to screenshot canvas %v: %v", params.resultID, err)
	}

	if expectedImg.Bounds() != resultImg.Bounds() {
		s.Fatalf("Mismatched image bounds: Expected %v, Actual %v", expectedImg.Bounds(),
			resultImg.Bounds())
	}

	// Compare colors of pixels from within each image quadrant to verify orientation. Note that
	// on non-square displays, the screenshots will have black margins. Correct for this by
	// positioning each sample relative to the image center, offset by 1/4 of the smaller image
	// dimension (which should generally be height in landscape mode).
	width := expectedImg.Bounds().Dx()
	height := expectedImg.Bounds().Dy()
	cX := width / 2
	cY := height / 2
	offset := height / 4
	if width < height {
		offset = width / 4
	}
	samples := []image.Point{
		{cX - offset, cY - offset},
		{cX - offset, cY + offset},
		{cX + offset, cY - offset},
		{cX + offset, cY + offset},
	}

	for i, sample := range samples {
		referenceColor := params.referenceColors[i]
		expectedColor := expectedImg.At(sample.X, sample.Y)
		if expectedColor != referenceColor {
			s.Errorf("Expected image did not match reference color at (%d, %d): "+
				"Expected %v, Actual %v",
				sample.X, sample.Y, referenceColor, expectedColor)
			continue
		}
		resultColor := resultImg.At(sample.X, sample.Y)
		if expectedColor != resultColor {
			s.Errorf("Mismatched colors at (%d, %d): Expected %v, Actual %v", sample.X,
				sample.Y, expectedColor, resultColor)
		}
	}
}
