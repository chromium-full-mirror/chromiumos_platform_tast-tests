// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"io/ioutil"

	ppb "chromiumos/system_api/printscanmgr_proto"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/printing/printer"
	"go.chromium.org/tast-tests/cros/local/printscanmgr"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// genericPPDFile is ppd.gz file to be registered via printscanmgr.
const httpTestPPDFilePrintscanmgr string = "printer_add_generic_printer_GenericPostScript.ppd.gz"

func init() {
	testing.AddTest(&testing.Test{
		Func:     AddHTTPPrinterPrintscanmgr,
		Desc:     "Verifies that HTTP printers can be installed using printscanmgr",
		Contacts: []string{"project-bolton@google.com", "pmoy@chromium.org"},
		// ChromeOS > Platform > baseOS > Printing
		BugComponent: "b:167231",
		SoftwareDeps: []string{"cups"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Data:         []string{httpTestPPDFilePrintscanmgr},
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
			"group:hw_agnostic",
		},
	})
}

func AddHTTPPrinterPrintscanmgr(ctx context.Context, s *testing.State) {
	// Downloads the PPD and tries to install the printer using the D-Bus method.
	const printerID = "HttpPrinterId"

	ppd, err := ioutil.ReadFile(s.DataPath(httpTestPPDFilePrintscanmgr))
	if err != nil {
		s.Fatal("Failed to read PPD file: ", err)
	}

	if err := printer.ResetCups(ctx, /*usePrintscanmgr=*/true); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	p, err := printscanmgr.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to printscanmgr: ", err)
	}

	testing.ContextLog(ctx, "Registering a printer")
	if result, err := p.CupsAddManuallyConfiguredPrinter(
		ctx,
		&ppb.CupsAddManuallyConfiguredPrinterRequest{
			Name:        printerID,
			Uri:         "http://chromium.org:999/this/is/a/test",
			PpdContents: ppd}); err != nil {
		s.Fatal("Failed to call CupsAddManuallyConfiguredPrinter: ", err)
	} else if result.Result != ppb.AddPrinterResult_ADD_PRINTER_RESULT_SUCCESS {
		s.Fatal("Could not set up a printer: ", result.Result)
	}
}
