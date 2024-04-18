// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package scanapp provides tests and utilities for the UI ScanApp.
package scanapp

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/scanapp/scanning"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/scanapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScanToDrive,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the Scan app can be used to perform scans to Drive",
		Contacts: []string{
			"cros-peripherals@google.com",
			"project-bolton@google.com",
			"gavinwill@google.com",
		},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_scanning",
			"group:cq-medium",
		},
		SoftwareDeps: []string{"chrome", "drivefs"},
		// TODO(b/202847398): Skip sona devices due to abnormal failures.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("sona")),
		Fixture:      "virtualUsbPrinterModulesLoadedWithDriveFsStarted",
		Data: []string{
			scanning.SourceImage,
			scanning.PNGGoldenFile,
			scanning.JPGGoldenFile,
			scanning.PDFGoldenFile,
		},
	})
}

var scanToDriveTests = []scanning.TestingStruct{
	{
		Name: "flatbed_png_color_letter_300_dpi",
		Settings: scanapp.ScanSettings{
			Source:     scanapp.SourceFlatbed,
			FileType:   scanapp.FileTypePNG,
			ColorMode:  scanapp.ColorModeColor,
			PageSize:   scanapp.PageSizeLetter,
			Resolution: scanapp.Resolution300DPI,
			ScanTo:     scanapp.MyDrive,
		},
		GoldenFile: scanning.PNGGoldenFile,
	}, {
		Name: "adf_simplex_jpg_grayscale_a4_150_dpi",
		Settings: scanapp.ScanSettings{
			Source:   scanapp.SourceADFOneSided,
			FileType: scanapp.FileTypeJPG,
			// TODO(b/181773386): Change this to black and white when the virtual
			// USB printer correctly reports the color mode.
			ColorMode:  scanapp.ColorModeGrayscale,
			PageSize:   scanapp.PageSizeA4,
			Resolution: scanapp.Resolution150DPI,
			ScanTo:     scanapp.MyDrive,
		},
		GoldenFile: scanning.JPGGoldenFile,
	}, {
		Name: "adf_duplex_pdf_grayscale_max_300_dpi",
		Settings: scanapp.ScanSettings{
			Source:     scanapp.SourceADFTwoSided,
			FileType:   scanapp.FileTypePDF,
			ColorMode:  scanapp.ColorModeGrayscale,
			PageSize:   scanapp.PageSizeFitToScanArea,
			Resolution: scanapp.Resolution300DPI,
			ScanTo:     scanapp.MyDrive,
		},
		GoldenFile: scanning.PDFGoldenFile,
	},
}

func ScanToDrive(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	cr := fixt.Chrome

	var scannerParams = scanning.ScannerStruct{
		Descriptors: scanning.Descriptors,
		Attributes:  scanning.Attributes,
		EsclCaps:    scanapp.EsclCapabilities,
	}

	scanning.RunAppSettingsTests(ctx, s, cr, scanToDriveTests, scannerParams)
}
