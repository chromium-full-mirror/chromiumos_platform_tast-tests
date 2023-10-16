// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RejectDocumentFormat,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that lpadmin handles a printer rejecting get-printer-attributes requests containing the document-format attribute",
		Contacts:     []string{"project-bolton@google.com", "pmoy@chromium.org"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
			"group:hw_agnostic",
		},
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{"cups"},
		Data:         []string{"reject_document_format_script.textproto"},
		Fixture:      "virtualUsbPrinterModulesLoaded",
	})
}

func RejectDocumentFormat(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	printer, err := usbprinter.Start(ctx,
		usbprinter.WithIPPUSBDescriptors(),
		usbprinter.WithGenericIPPAttributes(),
		usbprinter.WithMockPrinterScriptPath(s.DataPath("reject_document_format_script.textproto")))
	if err != nil {
		s.Fatal("Failed to start IPP-over-USB printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}(cleanupCtx)

	uri := fmt.Sprintf("ippusb://%s_%s/ipp/print", printer.DevInfo.VID, printer.DevInfo.PID)

	cmd := testexec.CommandContext(ctx, "lpadmin", "-v", uri, "-m", "everywhere")
	_, stderr, err := cmd.SeparatedOutput()
	// We don't expect lpadmin to succeed, since the mock printer is only
	// scripted to respond enough to test the desired behavior of this test.
	// Instead of success, we're looking for a specific exit code and a
	// corresponding error message that ensure lpadmin took the correct code
	// path.
	if code, ok := testexec.ExitCode(err); !ok || code != 9 {
		s.Fatal("Expected exit status 9 from `lpadmin`: ", err)
	}
	if !strings.Contains(string(stderr), "Failed to execute Get-Printer-Attributes request - retrying without document-format attribute") {
		s.Fatal("Error message did not contain `retrying without document-format`: ", string(stderr))
	}
}
