// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/scanapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScanApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that scanned files saved from Scan App appear in Holding Space",
		BugComponent: "b:1268276", // ChromeOS > Software > System UI Surfaces > HoldingSpace
		Contacts: []string{
			"tote-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"dmblack@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"cups", "chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-0d908149-1273-41b3-8c06-cb4ebf70892b",
		}},
		Fixture: "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
	})
}

// ScanApp tests the functionality of files existing in Holding Space by
// saving a scanned file from the Scan app.
func ScanApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Reset the holding space.
	if err := holdingspace.ResetHoldingSpace(ctx, tconn,
		holdingspace.ResetHoldingSpaceOptions{}); err != nil {
		s.Fatal("Failed to reset holding space: ", err)
	}

	printer, err := scanapp.StartPrinter(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to start printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}(ctx)

	var settings = scanapp.ScanSettings{
		ColorMode:  scanapp.ColorModeColor,
		FileType:   scanapp.FileTypePNG,
		PageSize:   scanapp.PageSizeLetter,
		Resolution: scanapp.Resolution300DPI,
		ScanTo:     scanapp.MyFiles,
		Scanner:    printer.VisibleName,
		Source:     scanapp.SourceFlatbed,
	}

	app, err := scanapp.LaunchAndStartScanWithSettings(ctx, tconn, settings)
	if err != nil {
		s.Fatal("Failed to Launch scan app and start scan: ", err)
	}
	defer func() {
		if err := app.Close(ctx); err != nil {
			s.Error("Failed to close app: ", err)
		}
	}()

	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve users MyFiles path: ", err)
	}

	// Remove scans after the test completes.
	defaultScanPattern := filepath.Join(myFilesPath, scanapp.DefaultScanFilePattern)
	defer func() {
		if err := scanapp.RemoveScans(defaultScanPattern); err != nil {
			s.Error("Failed to remove scans: ", err)
		}
	}()

	scan, err := scanapp.GetScan(defaultScanPattern)
	if err != nil {
		s.Fatal("Failed to find scan: ", err)
	}

	// Verify the scan can be found in the holding space.
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Verify scanned file appears in holding space",
		ui.LeftClick(holdingspace.FindTray()),
		ui.WaitUntilExists(holdingspace.FindDownloadChip().Name(filepath.Base(scan))),
	)(ctx); err != nil {
		s.Fatal("Failed to verify scanned file appears in holding space: ", err)
	}
}
