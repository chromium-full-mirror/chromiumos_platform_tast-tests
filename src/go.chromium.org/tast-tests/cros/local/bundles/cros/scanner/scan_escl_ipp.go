// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package scanner

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	lpb "go.chromium.org/chromiumos/system_api/lorgnette_proto"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/printing/ippusbbridge"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast-tests/cros/local/scanner/lorgnette"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type params struct {
	Network bool // Set up as a network device instead of local.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScanESCLIPP,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests eSCL scanning via an ipp-over-usb tunnel",
		Contacts:     []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_scanning",
		},
		SoftwareDeps: []string{"cups", "chrome"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
		Data:         []string{sourceImage, goldenImage},
		Params: []testing.Param{{
			Name: "usb",
			Val: &params{
				Network: false,
			},
		}, {
			Name: "network",
			Val: &params{
				Network: true,
			},
		}},
	})
}

const (
	sourceImage = "scan_escl_ipp_source.jpg"
	goldenImage = "scan_escl_ipp_golden.png"
)

func ScanESCLIPP(ctx context.Context, s *testing.State) {
	const esclCapabilities = "/usr/local/etc/virtual-usb-printer/escl_capabilities.json"

	testOpt := s.Param().(*params)

	// Use cleanupCtx for any deferred cleanups in case of timeouts or
	// cancellations on the shortened context.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	printer, err := usbprinter.Start(ctx,
		usbprinter.WithIPPUSBDescriptors(),
		usbprinter.WithGenericIPPAttributes(),
		usbprinter.WithESCLCapabilities(esclCapabilities),
		usbprinter.ExpectUdevEventOnStop(),
		usbprinter.WaitUntilConfigured())
	if err != nil {
		s.Fatal("Failed to attach virtual printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}(cleanupCtx)

	scanner, err := ippusbbridge.PrepareScannerConnection(ctx, printer.DevInfo, testOpt.Network)
	if err != nil {
		s.Fatal("Failed to prepare scanner connection: ", err)
	}
	defer scanner.Cleanup(cleanupCtx)

	tmpDir, err := ioutil.TempDir("", "tast.scanner.ScanEsclIPP.")
	if err != nil {
		s.Fatal("Failed to create temporary directory: ", err)
	}
	defer os.RemoveAll(tmpDir)

	startScanRequest := &lpb.StartScanRequest{
		DeviceName: scanner.ConnectionString,
		Settings: &lpb.ScanSettings{
			Resolution: 300,
			SourceName: "Flatbed",
			ColorMode:  lpb.ColorMode_MODE_COLOR,
		},
	}

	s.Log("Performing scan")
	scanPath, err := lorgnette.RunScan(ctx, startScanRequest, tmpDir)
	if err != nil {
		s.Fatal("Failed to run scan: ", err)
	}

	s.Log("Comparing scanned file to golden image")
	diff := testexec.CommandContext(ctx, "perceptualdiff", "-verbose", "-threshold", "1", scanPath, s.DataPath(goldenImage))
	if err := diff.Run(); err != nil {
		s.Error("Scanned file differed from golden image: ", err)
		diff.DumpLog(ctx)
	}

	// Intentionally stop the printer early to trigger shutdown in ippusb_bridge.
	// Without this, cleanup may have to wait for other processes to finish using
	// the printer (e.g. CUPS background probing).
	//
	// TODO(b/210134772): Investigate if this remains necessary.
	if err := printer.Stop(ctx); err != nil {
		s.Error("Failed to stop printer: ", err)
	}
}
