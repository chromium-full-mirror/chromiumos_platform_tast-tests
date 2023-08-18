// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package scanapp

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/scanapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LongScan,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the Scan app will not suspend during long scans",
		Contacts: []string{
			"cros-peripherals@google.com",
			"project-bolton@google.com",
		},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_scanning",
		},
		SoftwareDeps: []string{"chrome", "virtual_usb_printer"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
	})
}

// LongScan creates a delayed scan job and reduces the powerd sleep timeouts to
// test if the system will stay awake during a scan job.
func LongScan(ctx context.Context, s *testing.State) {
	// Use cleanupCtx for any deferred cleanups in case of timeouts or
	// cancellations on the shortened context.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_long_scan_tconn")

	printer, err := scanapp.StartPrinter(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to attach virtual printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}(cleanupCtx)

	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve users MyFiles path: ", err)
	}
	defaultScanPattern := filepath.Join(myFilesPath, scanapp.DefaultScanFilePattern)
	// Launch the Scan app, configure the settings, and perform scans.
	app, err := scanapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch app: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_long_scan_app_launch")
	defer func() {
		if err := scanapp.RemoveScans(defaultScanPattern); err != nil {
			s.Error("Failed to remove scans: ", err)
		}
	}()

	if err := app.ClickMoreSettings()(ctx); err != nil {
		s.Fatal("Failed to expand More settings: ", err)
	}

	// Setting Source = SourceFlatbed, ColorMode = ColorModeColor, and
	// Resolution = Resolution100DPI causes the scanner to delay 45 seconds
	// when retrieving the image for the scan job.
	// See https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/third_party/virtual-usb-printer/docs/scanning-special-cases.md
	if err := app.SetScanSettings(scanapp.ScanSettings{
		Scanner:    printer.VisibleName,
		Source:     scanapp.SourceFlatbed,
		FileType:   scanapp.FileTypePDF,
		ColorMode:  scanapp.ColorModeColor,
		PageSize:   scanapp.PageSizeLetter,
		Resolution: scanapp.Resolution100DPI,
		ScanTo:     scanapp.MyFiles,
	})(ctx); err != nil {
		s.Fatal("Failed to set scan settings: ", err)
	}

	// powerd keeps track of the number of times the system suspends and returns. If this
	// value is different before and after the scan, then we know the system suspended.
	initialWakeCount, err := suspend.WithoutRetries().Value()
	if err != nil {
		s.Fatal("Failed to get wake count: ", err)
	}

	// Make sure printer connected notifications don't cover the Scan button.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	// Tell OS to dim after 15 seconds of inactivity, suspend after 25.
	// See https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform2/power_manager/tools/set_short_powerd_timeouts
	s.Log("Decreasing power suspend timeout")
	suspendCmd := testexec.CommandContext(ctx, "set_short_powerd_timeouts")
	if err := suspendCmd.Run(); err != nil {
		s.Fatal("Failed to run powerd timeout command: ", err)
	}
	defer func() {
		// Reset OS suspend timeouts to be default.
		suspendCmd := testexec.CommandContext(ctx, "set_short_powerd_timeouts", "--reset")
		if err := suspendCmd.Run(); err != nil {
			s.Fatal("Failed to run powerd timeout reset: ", err)
		}
	}()

	s.Log("Starting Scan")
	if err := app.Scan()(ctx); err != nil {
		s.Fatal("Failed to perform scan: ", err)
	}

	finalWakeCount, err := suspend.WithoutRetries().Value()
	if err != nil {
		s.Fatal("Failed to get wake count: ", err)
	}

	if initialWakeCount != finalWakeCount {
		s.Fatal("Wake count is different, machine suspended")
	}

	if err := app.ClickDone()(ctx); err != nil {
		s.Fatal("Failed to complete scan: ", err)
	}
}
