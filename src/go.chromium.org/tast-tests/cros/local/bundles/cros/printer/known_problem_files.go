// Copyright 2023 The ChromiumOS Authors
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
		Func:         KnownProblemFiles,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests files that have been problematic in the past",
		Contacts:     []string{"project-bolton@google.com", "nmuggli@google.com"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
		},
		SoftwareDeps: []string{"chrome", "cups", "ghostscript"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
		Params: []testing.Param{
			{
				// This is a document with fillable fields that fails differently
				// depending on what cups-filters and ghostscript versions are
				// installed.  In some cases, pages up to page 7 get printed, page 7 is
				// blank, and the filter fails after that.  In other cases, all pages
				// get printed but some of the text is offset/mis-aligned from the
				// original document.
				Name: "fillable_fields",
				Val: usbprintertests.PrintJobSetup{
					ToPrint:     "Example_1.pdf",
					PrintedFile: "record.pwg",
					GoldenFile:  "Example_1_golden.pwg",
				},
				ExtraData: []string{"Example_1.pdf", "Example_1_golden.pwg"},
				// When rasterizing this file, there are a very small number of bits
				// that are different on arm32 boards.  We don't expect this test file
				// to cause regressions on just arm32 boards and not other boards, so
				// it's probably safe to skip boards with arm 32-bit userspace.
				ExtraSoftwareDeps: []string{"no_arm"},
			}, {
				// This is a Google Slides presentation that is printed from Google
				// Slides (which generates a PDF on the fly).  This causes a 'Filter
				// Failed' message.  Note that this file was created by modifying all
				// the cups filters in the pipeline to simply be a passthru and then
				// capturing the output sent to the printer.  This allows us to capture
				// the PDF generated from print preview when using Google Slides.  When
				// simply saving the slides presentation as a PDF document, the failed
				// filter is not reproducible.
				Name: "slides_presentation",
				Val: usbprintertests.PrintJobSetup{
					ToPrint:     "General_presentation.pdf",
					PrintedFile: "record.pwg",
					GoldenFile:  "General_presentation_golden.pwg",
				},
				ExtraData: []string{"General_presentation.pdf", "General_presentation_golden.pwg"},
			}, {
				// This is a test document with unembedded fonts.  This was failing to
				// find the correct fonts to substitute.  It can also be used to verify
				// that font substitution works in general.
				Name: "unembedded_fonts",
				Val: usbprintertests.PrintJobSetup{
					ToPrint:     "test_fonts_unembedded.pdf",
					PrintedFile: "record.pwg",
					GoldenFile:  "test_fonts_unembedded_golden.pwg",
				},
				ExtraData: []string{"test_fonts_unembedded.pdf", "test_fonts_unembedded_golden.pwg"},
			}, {
				// This is a test document with unembedded ZapfDingbats font.  For
				// awhile we did not have dingbats font aliased correctly - this checks
				// for that.
				Name: "unembedded_dingbats_font",
				Val: usbprintertests.PrintJobSetup{
					ToPrint:     "test_fonts_unembedded_dingbats.pdf",
					PrintedFile: "record.pwg",
					GoldenFile:  "test_fonts_unembedded_dingbats_golden.pwg",
				},
				ExtraData: []string{"test_fonts_unembedded_dingbats.pdf", "test_fonts_unembedded_dingbats_golden.pwg"},
			},
		},
	})
}

func KnownProblemFiles(ctx context.Context, s *testing.State) {
	param := s.Param().(usbprintertests.PrintJobSetup)

	tmpDir, err := ioutil.TempDir("", "tast.printer.KnownProblemFiles.")
	if err != nil {
		s.Fatal("Failed to create temporary directory")
	}
	defer os.RemoveAll(tmpDir)
	recordPath := filepath.Join(tmpDir, param.PrintedFile)

	usbprintertests.RunPrintTest(ctx, s,
		[]usbprinter.Option{
			usbprinter.WithIPPUSBDescriptors(),
			usbprinter.WithAttributes("ipp_attributes_pwgraster.json"),
			usbprinter.WithRecordPath(recordPath),
			usbprinter.WaitUntilConfigured(),
		},
		"", // PPD
		usbprintertests.PrintJobSetup{
			PrintedFile: recordPath,
			ToPrint:     s.DataPath(param.ToPrint),
			GoldenFile:  s.DataPath(param.GoldenFile),
		})
}
