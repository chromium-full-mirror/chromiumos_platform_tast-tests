// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPreviewOCR,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies OCR scanning on preview",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Data:         []string{"ocr_one_line_3264x2448.jpg"},
		Fixture:      "ccaTestBridgeReadyWithFakeHALCameraWithPreviewOCR",
	})
}

func CCAUIPreviewOCR(ctx context.Context, s *testing.State) {
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene
	cr := s.FixtValue().(cca.FixtureData).Chrome
	if err := switchScene(ctx, cca.SceneData{Path: s.DataPath("ocr_one_line_3264x2448.jpg"), ScaleMode: "contain"}); err != nil {
		s.Fatal("Failed to setup scene: ", err)
	}
	if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
		// TODO(b/335104005): Check more scenarios: no text, multiple-line text, and text plus barcode.
		return testPreviewOCR(ctx, app, cr, "hello.")
	}, cca.TestWithAppParams{}); err != nil {
		s.Error("Failed to run test: ", err)
	}

}

func testPreviewOCR(ctx context.Context, app *cca.App, cr *chrome.Chrome, expectedText string) error {
	// Barcode and OCR use the same components to show and copy detected text.
	if err := app.WaitForVisibleStateFor(ctx, cca.BarcodeChipText, true, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to detect text")
	}
	if err := app.Click(ctx, cca.BarcodeCopyTextButton); err != nil {
		return errors.Wrap(err, "failed to click copy button")
	}
	// Check for the snack bar to indicate the text has been copied.
	if err := app.WaitForVisibleState(ctx, cca.Snackbar, true); err != nil {
		return errors.Wrap(err, "failed to show snack bar")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test connection")
	}
	if err := ash.WaitUntilClipboardText(ctx, tconn, expectedText); err != nil {
		return errors.Wrap(err, "failed to copy detected text")
	}

	return nil
}
