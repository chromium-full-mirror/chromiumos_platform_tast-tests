// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lpprint implements adding a printer, printing to it via the lp command,
// and comparing the data sent to the printer to a golden file.
package lpprint

import (
	"context"
	"io/ioutil"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/fake"
	"go.chromium.org/tast-tests/cros/local/debugd"
	"go.chromium.org/tast-tests/cros/local/printing/printer"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Run runs the lp command and returns the generated print output.
func Run(ctx context.Context, ppdFilePath, toPrintFilePath, options string) ([]byte, error) {
	const printerID = "FakePrinterID"

	ppd, err := ioutil.ReadFile(ppdFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read PPD file")
	}

	if err := printer.ResetCups(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reset cupsd")
	}

	fake, err := fake.NewPrinter(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start fake printer")
	}
	defer fake.Close()

	d, err := debugd.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to debugd")
	}

	testing.ContextLog(ctx, "Registering a printer")
	if result, err := d.CupsAddManuallyConfiguredPrinter(ctx, printerID, "socket://localhost:9101", ppd); err != nil {
		return nil, errors.Wrap(err, "debugd.CupsAddManuallyConfiguredPrinter failed")
	} else if result != debugd.CUPSSuccess {
		return nil, errors.Errorf("could not set up a printer: %v", result)
	}

	testing.ContextLog(ctx, "Issuing print request")
	args := []string{"-d", printerID}
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
