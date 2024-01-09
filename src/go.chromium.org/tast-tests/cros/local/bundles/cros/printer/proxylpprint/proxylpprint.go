// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package proxylpprint implements adding a printer, printing to it via the lp command,
// and comparing the data sent to the printer to a golden file.
package proxylpprint

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/fake"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/uitools"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/printing/printer"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Run runs the lp command and returns the generated print output.
func Run(ctx context.Context, chrome *chrome.Chrome, ppdFilePath, toPrintFilePath, options string) (_ []byte, returnErr error) {
	const printerID = "FakePrinterID"

	err := upstart.EnsureJobRunning(ctx, "cups_proxy")
	if err != nil {
		return nil, errors.Wrap(err, "failed to start cups_proxy service")
	}

	if _, err := os.Stat(ppdFilePath); err != nil {
		return nil, errors.Wrap(err, "failed to read PPD file")
	}

	if err := printer.ResetCups(ctx, /*usePrintscanmgr=*/false); err != nil {
		return nil, errors.Wrap(err, "failed to reset cupsd")
	}

	fake, err := fake.NewPrinter(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start fake printer")
	}
	defer fake.Close()

	tconn, err := chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	testing.ContextLog(ctx, "Registering a printer")
	printerInfo := uitools.PrinterInfo{
		DisplayName: printerID,
		ID:          printerID,
		URI:         "socket://localhost:9101/",
		PPDPath:     ppdFilePath,
	}
	if err := uitools.AddOrUpdatePrinter(ctx, tconn, printerInfo); err != nil {
		return nil, errors.Wrap(err, "autotestPrivate.updatePrinter() failed")
	}

	defer func() {
		err := tconn.Call(ctx, nil, "chrome.autotestPrivate.removePrinter", printerID)
		if err != nil {
			if returnErr != nil {
				returnErr = errors.Wrapf(returnErr, "autotestPrivate.removePrinter() failed: %v", err)
			} else {
				returnErr = errors.Wrap(err, "autotestPrivate.removePrinter() failed")
			}
		}
	}()

	testing.ContextLog(ctx, "Issuing print request")
	args := []string{"-h", "/run/cups_proxy/cups.sock", "-d", printerID}
	if len(options) != 0 {
		args = append(args, "-o", options)
	}
	args = append(args, toPrintFilePath)
	cmd := testexec.CommandContext(ctx, "lp", args...)

	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return nil, errors.Wrap(err, "failed to run lp")
	}

	testing.ContextLog(ctx, "Receiving print request")
	recvCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := fake.ReadRequest(recvCtx)
	if err != nil {
		return nil, errors.Wrap(err, "fake printer didn't receive a request")
	}
	return request, nil
}
