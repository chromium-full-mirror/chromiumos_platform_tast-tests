// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package usbprintertests provides utility functions for running tests with a
// USB printer.
package usbprintertests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/printing/document"
	"go.chromium.org/tast-tests/cros/local/printing/lp"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func usbPrinterURI(ctx context.Context, devInfo usbprinter.DevInfo) string {
	return fmt.Sprintf("usb://%s/%s", devInfo.VID, devInfo.PID)
}

// PrintJobSetup describes a file to print, where to find the output, and what
// the output should look like.
type PrintJobSetup struct {
	ToPrint     string
	PrintedFile string
	GoldenFile  string
}

// RunPrintTest executes a test for the virtual USB printer defined by the
// given arguments. This tests that the printer is able to be configured, and
// produces the expected output at record when a print job is issued.
// toPrint and golden specify the paths to the file to be printed and the
// expected printer output.
func RunPrintTest(ctx context.Context, s *testing.State,
	opts []usbprinter.Option, ppd string, job PrintJobSetup) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if cr, ok := s.FixtValue().(chrome.HasChrome); ok {
		tconn, err := cr.Chrome().TestAPIConn(cleanupCtx)
		if err != nil {
			s.Fatal("Failed to create Test API connection: ", err)
		}
		defer faillog.DumpDeviceEventLogOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "printer,usb")
	}

	pr, err := usbprinter.Start(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to attach virtual printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := pr.Stop(ctx); err != nil {
			s.Error("Failed to stop virtual printer: ", err)
		}
		// Remove the recorded file. Ignore errors if the path doesn't exist.
		if err := os.Remove(job.PrintedFile); err != nil && !os.IsNotExist(err) {
			s.Error("Failed to remove file: ", err)
		}
	}(cleanupCtx)

	// If no PPD was provided, then the printer is an IPP-over-USB device
	// for which we waited on autoconf; the name will have been stored in
	// the PrinterInstance.
	var foundPrinterName = pr.ConfiguredName
	if ppd != "" {
		// If a PPD is provided then we configure the printer ourselves.
		foundPrinterName = "virtual-test"
		if err := lp.CupsAddPrinter(ctx, foundPrinterName, usbPrinterURI(ctx, pr.DevInfo), ppd); err != nil {
			s.Fatal("Failed to configure printer: ", err)
		}
	}
	s.Log("Printer configured with name: ", foundPrinterName)

	RunPrintJob(ctx, s, foundPrinterName, job)
}

// RunPrintJob sends toPrint to the printer called printerName.  After the job
// has completed, it reads the output from printedFile and compares it to
// goldenFile.
func RunPrintJob(ctx context.Context, s *testing.State, printerName string, job PrintJobSetup) {
	jobID, err := lp.CupsStartPrintJob(ctx, printerName, job.ToPrint)
	if err != nil {
		s.Fatal("Failed to start printer: ", err)
	}

	s.Logf("Waiting for %s to complete", jobID)
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		if done, err := lp.JobCompleted(ctx, printerName, jobID); err != nil {
			return err
		} else if !done {
			return errors.Errorf("job %s is not done yet", jobID)
		}
		testing.ContextLogf(ctx, "Job %s is complete", jobID)
		return nil
	}, nil); err != nil {
		s.Fatal("Print job didn't complete: ", err)
	}
	if err = document.CompareFiles(ctx, job.PrintedFile, job.GoldenFile,
		s.OutDir(), "diff.txt", filepath.Base(job.GoldenFile)); err != nil {
		s.Error("Printer output differs from expected: ", err)
	}
}
