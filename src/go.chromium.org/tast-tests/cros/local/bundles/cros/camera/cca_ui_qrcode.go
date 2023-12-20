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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type qrcodeTestParams struct {
	format      string
	scene       string
	barcodeChip *barcodeChip
	copyButton  *copyButton
	wifiConfig  *wifiConfig
}

type barcodeChip struct {
	component cca.UIComponentName
	clickable bool
}

type copyButton struct {
	button   cca.UIComponentName
	expected string
}

type wifiConfig struct {
	ssid     string
	password string
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
		Data:         []string{"qrcode_1280x960.mjpeg", "qrcode_text_1280x960.mjpeg", "qrcode_wifi_1280x960_20231225.jpg"},
		Params: []testing.Param{{
			Fixture: "ccaTestBridgeReadyWithAutoQROnFakeHALCamera",
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "ccaTestBridgeReadyWithAutoQROnFakeHALCameraLacros",
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
			format:      "url",
			barcodeChip: &barcodeChip{component: cca.BarcodeChipURL, clickable: true},
			copyButton: &copyButton{
				button:   cca.BarcodeCopyURLButton,
				expected: "https://www.google.com/chromebook/chrome-os/",
			},
			wifiConfig: nil,
		},
	}, {
		"text",
		"qrcode_text_1280x960.mjpeg",
		qrcodeTestParams{
			format:      "text",
			barcodeChip: &barcodeChip{component: cca.BarcodeChipText, clickable: false},
			copyButton: &copyButton{
				button:   cca.BarcodeCopyTextButton,
				expected: "ChromeOS is the speedy, simple and secure operating system that powers every Chromebook.",
			},
			wifiConfig: nil,
		},
	}, {
		"wifi",
		"qrcode_wifi_1280x960_20231225.jpg",
		qrcodeTestParams{
			format:      "wi-fi",
			barcodeChip: &barcodeChip{component: cca.BarcodeChipWifi, clickable: true},
			copyButton:  nil,
			wifiConfig: &wifiConfig{
				ssid:     "testnetwork",
				password: "testpassword",
			},
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

	if err := app.WaitForVisibleState(ctx, testParams.barcodeChip.component, true); err != nil {
		return errors.Wrapf(err, "failed to detect %v from barcode", testParams.format)
	}
	testing.ContextLogf(ctx, "%v detected", testParams.format)

	if testParams.copyButton != nil {
		// Copy the content.
		if err := app.Click(ctx, testParams.copyButton.button); err != nil {
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
			if clipData != testParams.copyButton.expected {
				return errors.Errorf("unexpected clipboard data: got %q, want %q", clipData, testParams.copyButton.expected)
			}
			testing.ContextLogf(ctx, "%v copied successfully", testParams.format)
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to get expected clipboard data")
		}
	}

	if testParams.barcodeChip.clickable {
		if err := app.Click(ctx, testParams.barcodeChip.component); err != nil {
			return errors.Wrap(err, "failed to click chip")
		}

		if testParams.format == "url" {
			br, brCleanUp, err := browserfixt.Connect(ctx, cr, bt)
			if err != nil {
				return errors.Wrap(err, "failed to connect to browser")
			}
			defer brCleanUp(ctx)

			if err := testing.Poll(ctx, func(ctx context.Context) error {
				ok, err := br.IsTargetAvailable(ctx, chrome.MatchTargetURL(testParams.copyButton.expected))
				if err != nil {
					return testing.PollBreak(err)
				}
				if !ok {
					return errors.Errorf("no match target for %v", testParams.copyButton.expected)
				}
				testing.ContextLogf(ctx, "%v opened successfully", testParams.format)
				return nil
			}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
				return errors.Wrap(err, "failed to open")
			}
		} else if testParams.format == "wi-fi" {
			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to connect to Chrome")
			}

			ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

			ssidFinder := nodewith.Role(role.StaticText).Name(testParams.wifiConfig.ssid)
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(ssidFinder)(ctx); err != nil {
				return errors.Wrap(err, "failed to find SSID in a dialog")
			}

			visibleButton := nodewith.Role(role.Button).ClassName("icon-visibility")
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(visibleButton)(ctx); err != nil {
				return errors.Wrap(err, "failed to find the password visibility button")
			}
			if err := ui.LeftClick(visibleButton)(ctx); err != nil {
				return errors.Wrap(err, "failed to left click of the password visibility button")
			}

			passwordFinder := nodewith.Role(role.StaticText).Name(testParams.wifiConfig.password)
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(passwordFinder)(ctx); err != nil {
				return errors.Wrap(err, "failed to find password in a dialog")
			}

			connectButton := nodewith.Role(role.Button).Name("Connect")
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(connectButton)(ctx); err != nil {
				return errors.Wrap(err, "failed to find the network connection button")
			}
			if err := ui.CheckRestriction(connectButton, restriction.None)(ctx); err != nil {
				return errors.Wrap(err, "failed to check if the network connection button is enabled")
			}
		}
	}
	// TODO(b/172879638): Test invalid binary content.
	return nil
}
