// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package scanapp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/scanapp/scanning"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/scanapp"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LargePaperScans,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the Scan app supports large paper size selection when available from printer",
		Contacts:     []string{"project-bolton@google.com", "cros-peripherals@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_scanning",
		},
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"cups", "chrome"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
		Data: []string{
			scanning.SourceImage,
			a3GoldenFile,
			a4GoldenFile,
			b4GoldenFile,
			legalGoldenFile,
			letterGoldenFile,
			tabloidGoldenFile,
		},
	})
}

const (
	esclCapabilities  = "/usr/local/etc/virtual-usb-printer/escl_capabilities_large_paper_sizes.json"
	a3GoldenFile      = "a3_golden_file.png"
	a4GoldenFile      = "a4_golden_file.png"
	b4GoldenFile      = "b4_golden_file.png"
	legalGoldenFile   = "legal_golden_file.png"
	letterGoldenFile  = "letter_golden_file.png"
	tabloidGoldenFile = "tabloid_golden_file.png"
)

var testSetups = []scanning.TestingStruct{
	{
		Name: "paper_size_a3",
		Settings: scanapp.ScanSettings{
			Source:     scanapp.SourceFlatbed,
			FileType:   scanapp.FileTypePNG,
			ColorMode:  scanapp.ColorModeColor,
			PageSize:   scanapp.PageSizeA3,
			Resolution: scanapp.Resolution300DPI,
			ScanTo:     scanapp.MyFiles,
		},
		GoldenFile: a3GoldenFile,
	}, {
		Name: "paper_size_a4",
		Settings: scanapp.ScanSettings{
			Source:     scanapp.SourceFlatbed,
			FileType:   scanapp.FileTypePNG,
			ColorMode:  scanapp.ColorModeColor,
			PageSize:   scanapp.PageSizeA4,
			Resolution: scanapp.Resolution300DPI,
			ScanTo:     scanapp.MyFiles,
		},
		GoldenFile: a4GoldenFile,
	}, {
		Name: "paper_size_b4",
		Settings: scanapp.ScanSettings{
			Source:     scanapp.SourceFlatbed,
			FileType:   scanapp.FileTypePNG,
			ColorMode:  scanapp.ColorModeColor,
			PageSize:   scanapp.PageSizeB4,
			Resolution: scanapp.Resolution300DPI,
			ScanTo:     scanapp.MyFiles,
		},
		GoldenFile: b4GoldenFile,
	}, {
		Name: "paper_size_legal",
		Settings: scanapp.ScanSettings{
			Source:     scanapp.SourceFlatbed,
			FileType:   scanapp.FileTypePNG,
			ColorMode:  scanapp.ColorModeColor,
			PageSize:   scanapp.PageSizeLegal,
			Resolution: scanapp.Resolution300DPI,
			ScanTo:     scanapp.MyFiles,
		},
		GoldenFile: legalGoldenFile,
	}, {
		Name: "paper_size_letter",
		Settings: scanapp.ScanSettings{
			Source:     scanapp.SourceFlatbed,
			FileType:   scanapp.FileTypePNG,
			ColorMode:  scanapp.ColorModeColor,
			PageSize:   scanapp.PageSizeLetter,
			Resolution: scanapp.Resolution300DPI,
			ScanTo:     scanapp.MyFiles,
		},
		GoldenFile: letterGoldenFile,
	}, {
		Name: "paper_size_tabloid",
		Settings: scanapp.ScanSettings{
			Source:     scanapp.SourceFlatbed,
			FileType:   scanapp.FileTypePNG,
			ColorMode:  scanapp.ColorModeColor,
			PageSize:   scanapp.PageSizeTabloid,
			Resolution: scanapp.Resolution300DPI,
			ScanTo:     scanapp.MyFiles,
		},
		GoldenFile: tabloidGoldenFile,
	},
}

func LargePaperScans(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	var scannerParams = scanning.ScannerStruct{
		Descriptors: scanning.Descriptors,
		Attributes:  scanning.Attributes,
		EsclCaps:    esclCapabilities,
	}

	scanning.RunAppSettingsTests(ctx, s, cr, testSetups, scannerParams)
}
