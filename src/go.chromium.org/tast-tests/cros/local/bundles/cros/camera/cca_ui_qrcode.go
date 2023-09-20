// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type qrcodeTestParams struct {
	format     string
	expected   string
	scene      string
	chip       cca.UIComponentName
	copyButton cca.UIComponentName
	canOpen    bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIQRCode,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks QR code detection in CCA",
		Contacts:     []string{"chromeos-camera-eng@google.com", "shik@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome", "chrome_internal"},
		Data:         []string{"qrcode_1280x960.mjpeg", "qrcode_text_1280x960.mjpeg"},
		Params: []testing.Param{{
			Fixture: "ccaTestBridgeReadyWithFakeHALCamera",
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "ccaTestBridgeReadyWithFakeHALCameraLacros",
		}},
	})
}

// CCAUIQRCode verifies that QR code scanning feature in CCA works.
func CCAUIQRCode(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(cca.FixtureData).Chrome
	bt := s.FixtValue().(cca.FixtureData).BrowserType
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene

	subTestTimeout := 30 * time.Second
	for _, tst := range []struct {
		name       string
		scene      string
		testParams qrcodeTestParams
	}{{
		"url",
		"qrcode_1280x960.mjpeg",
		qrcodeTestParams{
			format:     "url",
			expected:   "https://www.google.com/chromebook/chrome-os/",
			chip:       cca.BarcodeChipURL,
			copyButton: cca.BarcodeCopyURLButton,
			canOpen:    true,
		},
	}, {
		"text",
		"qrcode_text_1280x960.mjpeg",
		qrcodeTestParams{
			format:     "text",
			expected:   "ChromeOS is the speedy, simple and secure operating system that powers every Chromebook.",
			chip:       cca.BarcodeChipText,
			copyButton: cca.BarcodeCopyTextButton,
			canOpen:    false,
		},
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := switchScene(ctx, cca.SceneData{Path: s.DataPath(tst.scene), ScaleMode: "contain"}); err != nil {
				s.Fatal("Failed to setup QRCode scene: ", err)
			}
			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				return runQRCodeTest(ctx, cr, bt, app, tst.testParams)
			}, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}
}

func runQRCodeTest(ctx context.Context, cr *chrome.Chrome, bt browser.Type, app *cca.App, testParams qrcodeTestParams) error {
	if err := app.OpenQRCodeScanMode(ctx); err != nil {
		return errors.Wrap(err, "failed to open QR code scan mode")
	}
	testing.ContextLog(ctx, "Start scanning QR Code")

	if err := app.WaitForVisibleState(ctx, testParams.chip, true); err != nil {
		return errors.Wrapf(err, "failed to detect %v from barcode", testParams.format)
	}
	testing.ContextLogf(ctx, "%v detected", testParams.format)

	// Copy the content.
	if err := app.Click(ctx, testParams.copyButton); err != nil {
		return errors.Wrap(err, "failed to click copy button")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test connection")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var clipData string
		if err := tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.getClipboardTextData)()`, &clipData); err != nil {
			return testing.PollBreak(err)
		}
		if clipData != testParams.expected {
			return errors.Errorf("unexpected clipboard data: got %q, want %q", clipData, testParams.expected)
		}
		testing.ContextLogf(ctx, "%v copied successfully", testParams.format)
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to get expected clipboard data")
	}

	if testParams.canOpen {
		if err := app.Click(ctx, testParams.chip); err != nil {
			return errors.Wrap(err, "failed to click chip")
		}

		br, brCleanUp, err := browserfixt.Connect(ctx, cr, bt)
		if err != nil {
			return errors.Wrap(err, "failed to connect to browser")
		}
		defer brCleanUp(ctx)

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			ok, err := br.IsTargetAvailable(ctx, chrome.MatchTargetURL(testParams.expected))
			if err != nil {
				return testing.PollBreak(err)
			}
			if !ok {
				return errors.Errorf("no match target for %v", testParams.expected)
			}
			testing.ContextLogf(ctx, "%v opened successfully", testParams.format)
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to open")
		}
	}
	// TODO(b/172879638): Test invalid binary content.
	return nil
}
