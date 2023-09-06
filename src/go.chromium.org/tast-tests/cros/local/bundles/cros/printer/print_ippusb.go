// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/usbprintertests"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintIPPUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests ipp-over-usb printing",
		Contacts:     []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
		},
		SoftwareDeps: []string{"cups", "virtual_usb_printer"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Data:         []string{"print_ippusb_to_print.pdf", "print_ippusb_golden.pdf"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
	})
}

func PrintIPPUSB(ctx context.Context, s *testing.State) {
	tmpDir, err := ioutil.TempDir("", "tast.printer.PrintIPPUSB.")
	if err != nil {
		s.Fatal("Failed to create temporary directory")
	}
	defer os.RemoveAll(tmpDir)
	recordPath := filepath.Join(tmpDir, "record.pdf")

	usbprintertests.RunPrintTest(ctx, s,
		[]usbprinter.Option{
			usbprinter.WithIPPUSBDescriptors(),
			usbprinter.WithGenericIPPAttributes(),
			usbprinter.WithRecordPath(recordPath),
			usbprinter.WaitUntilConfigured(),
		},
		"", // PPD
		usbprintertests.PrintJobSetup{
			PrintedFile: recordPath,
			ToPrint:     s.DataPath("print_ippusb_to_print.pdf"),
			GoldenFile:  s.DataPath("print_ippusb_golden.pdf"),
		})
}
