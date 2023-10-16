// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/usbprintertests"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IPPUSBPPDCopiesUnsupported,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the 'copies-supported' attribute of the printer is used to populate the cupsManualCopies and cupsMaxCopies values in the corresponding generated PPD",
		Contacts:     []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
		},
		SoftwareDeps: []string{"cups"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Data:         []string{"ippusb_copies_unsupported.json"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
	})
}

// IPPUSBPPDCopiesUnsupported tests that the "cupsManualCopies" and
// "cupsMaxCopies" PPD fields will be correctly populated when configuring an
// IPP-over-USB printer whose "copies-supported" IPP attribute has an upper
// limit of 1 (i.e., it does not support copies).
func IPPUSBPPDCopiesUnsupported(ctx context.Context, s *testing.State) {
	usbprintertests.RunIPPUSBPPDTest(ctx, s, s.DataPath("ippusb_copies_unsupported.json"), map[string]string{
		"*cupsManualCopies": "True",
		"*cupsMaxCopies":    "1",
	})
}
