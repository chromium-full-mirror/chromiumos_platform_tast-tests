// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"

	"chromiumos/tast/local/bundles/cros/printer/pre"
	"chromiumos/tast/local/printing/usbprinter"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     AddUSBPrinter,
		Desc:     "Verifies setup of a basic USB printer",
		Contacts: []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
		},
		SoftwareDeps: []string{"cups", "virtual_usb_printer"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Fixture:      "virtualUsbPrinterModulesLoaded",
	})
}

func AddUSBPrinter(ctx context.Context, s *testing.State) {
	pr, err := usbprinter.Start(ctx,
		usbprinter.WithDescriptors("usb_printer.json"))
	if err != nil {
		s.Fatal("Failed to attach virtual printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := pr.Stop(ctx); err != nil {
			s.Error("Failed to stop virtual printer: ", err)
		}
	}(ctx)
}
