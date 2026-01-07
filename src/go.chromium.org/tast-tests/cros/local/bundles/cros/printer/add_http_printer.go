// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"os"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/debugd"
	"go.chromium.org/tast-tests/cros/local/printing/printer"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// genericPPDFile is ppd.gz file to be registered via debugd.
const httpTestPPDFile string = "printer_add_generic_printer_GenericPostScript.ppd.gz"

func init() {
	testing.AddTest(&testing.Test{
		Func:     AddHTTPPrinter,
		Desc:     "Verifies that http printers can be installed",
		Contacts: []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		SoftwareDeps: []string{"cups"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Data:         []string{httpTestPPDFile},
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
		},
	})
}

func AddHTTPPrinter(ctx context.Context, s *testing.State) {
	// Downloads the PPD and tries to install the printer using the dbus method.
	const printerID = "HttpPrinterId"

	ppd, err := os.ReadFile(s.DataPath(httpTestPPDFile))
	if err != nil {
		s.Fatal("Failed to read PPD file: ", err)
	}

	if err := printer.ResetCups(ctx, true /*usePrintscanmgr*/); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	d, err := debugd.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to debugd: ", err)
	}

	testing.ContextLog(ctx, "Registering a printer")
	if result, err := d.CupsAddManuallyConfiguredPrinter(
		ctx, printerID, "http://chromium.org:999/this/is/a/test", ppd); err != nil {
		s.Fatal("Failed to call CupsAddManuallyConfiguredPrinter: ", err)
	} else if result != debugd.CUPSSuccess {
		s.Fatal("Could not set up a printer: ", result)
	}
}
