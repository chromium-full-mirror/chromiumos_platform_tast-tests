// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chromeprintingsvc contains shared utility functions for ChromePrintingService Tast tests.
package chromeprintingsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "go.chromium.org/tast-tests/cros/services/cros/printer"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

// FormatPrinterDetails converts a slice of pb.printer objects into a newline-separated
// string, with each line containing the name and URI of a printer.
// It is primarily used for logging available printers when an error occurs.
func FormatPrinterDetails(printers []*pb.Printer) string {
	var printerDetails []string
	for _, p := range printers {
		printerDetails = append(printerDetails, fmt.Sprintf("Name: %q, URI: %q", p.Name, p.Uri))
	}
	return strings.Join(printerDetails, "\n")
}

// FindTargetPrinterID retrieves the printer ID of a given target printer name.
//
// It first reads the target printer name from the "printer.targetPrinterName"
// test variable, then polls GetPrinters on the service until a
// matching printer is found.
//
// An error is returned if the test variable is missing or the printer is
// not discovered within the polling timeout.
func FindTargetPrinterID(ctx context.Context, svc pb.ChromePrintingServiceClient, targetPrinterName string) (string, error) {
	// Initial printers to log in case printer.targetPrinterName is empty.
	emptyReq := &emptypb.Empty{}
	initialPrinters, err := svc.GetPrinters(ctx, emptyReq)
	if err != nil {
		return "", errors.Wrap(err, "failed to call GetPrinters")
	}

	if targetPrinterName == "" {
		formattedPrinters := FormatPrinterDetails(initialPrinters.Printers)
		return "", errors.Errorf("Parameter 'printer.targetPrinterName' is missing. Available printers: %s", "\n"+formattedPrinters)
	}

	var getPrintersResp *pb.GetPrintersResponse
	var printerID string

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		getPrintersResp, err = svc.GetPrinters(ctx, emptyReq)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to call getPrinters"))
		}

		for _, p := range getPrintersResp.Printers {
			if strings.Contains(p.Name, targetPrinterName) {
				printerID = p.Id
				return nil
			}
		}
		return errors.New("target Printer not discovered")
	}, &testing.PollOptions{
		// Arbitrary, enough time for tast to initiate and start looking printers.
		Timeout:  100 * time.Second,
		Interval: 5 * time.Second,
	}); err != nil {
		formattedPrinters := FormatPrinterDetails(getPrintersResp.Printers)
		return "", errors.Wrapf(err, "failed to find printer %q. Available printers: %s", targetPrinterName, "\n"+formattedPrinters)
	}

	return printerID, nil
}
