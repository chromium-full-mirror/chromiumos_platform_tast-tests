// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lp provides an API for interacting with the CUPS daemon on ChromeOS
// via lp/lpstat/lpamin/etc.
package lp

import (
	"context"
	"os"
	"regexp"
	"strings"

	ppb "go.chromium.org/chromiumos/system_api/printscanmgr_proto"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/printscanmgr"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Regular expression used to match a line from the output of the lpstat
// command.
const lpstatPatternPrefix = `device for ([-a-zA-Z0-9]+): `

// PrinterNameByURI runs the lpstat command to search for a configured printer
// which corresponds to uri. Return the name of the matching printer if found.
func PrinterNameByURI(ctx context.Context, uri string) (name string, err error) {
	out, stderr, err := testexec.CommandContext(ctx, "lpstat", "-t").SeparatedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "failed to run scan for configured printers; %s", stderr)
	}

	r := regexp.MustCompile(lpstatPatternPrefix + regexp.QuoteMeta(uri))
	for _, line := range strings.Split(string(out), "\n") {
		submatches := r.FindStringSubmatch(line)
		if submatches != nil {
			name := submatches[1]

			// check if the printer is idle and ready to accept jobs
			lr := regexp.MustCompile(name + " accepting requests since")
			for _, nline := range strings.Split(string(out), "\n") {
				idleSubmatches := lr.FindStringSubmatch(nline)
				if idleSubmatches != nil {
					return name, nil
				}
			}
		}
	}

	return "", errors.Errorf("failed to find printer with uri %s", uri)
}

// CupsAddPrinter adds a new printer using CUPS. Returns an error if the ppd
// is empty or lpadmin fails.
func CupsAddPrinter(ctx context.Context, printerName, uri, ppd string) error {
	if ppd == "" {
		return errors.New("must provide PPD to CupsAddPrinter")
	}
	ppdContents, err := os.ReadFile(ppd)
	if err != nil {
		return errors.Wrap(err, "failed to read PPD file")
	}
	p, err := printscanmgr.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to printscanmgr")
	}
	testing.ContextLog(ctx, "Adding printer to CUPS using ", uri)
	if result, err := p.CupsAddManuallyConfiguredPrinter(
		ctx,
		&ppb.CupsAddManuallyConfiguredPrinterRequest{
			Name:        printerName,
			Uri:         uri,
			PpdContents: ppdContents}); err != nil {
		return errors.Wrap(err, "printscanmgr.CupsAddManuallyConfiguredPrinter failed")
	} else if result.Result != ppb.AddPrinterResult_ADD_PRINTER_RESULT_SUCCESS {
		return errors.Errorf("could not set up a printer: %v", result)
	}
	return nil
}

// CupsRemovePrinter removes the printer that was configured for testing.
func CupsRemovePrinter(ctx context.Context, printerName string) error {
	p, err := printscanmgr.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to printscanmgr")
	}
	_, err = p.CupsRemovePrinter(
		ctx,
		&ppb.CupsRemovePrinterRequest{
			Name: printerName})
	return err
}

// CupsStartPrintJob starts a new print job for the file toPrint. This function
// adds '-d printerName' and the name of the file as args to the lp command.  If
// the user wants any additional args to the lp command, they need to get
// populated in options.  Returns the ID of the newly created job if successful.
func CupsStartPrintJob(ctx context.Context, printerName, toPrint string, options ...string) (job string, err error) {
	testing.ContextLog(ctx, "Starting print job")
	options = append(options, "-d", printerName, "--", toPrint)
	output, err := testexec.CommandContext(ctx, "lp", options...).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}

	// Example output from lp command: "request id is MyPrinter-32"
	// In this case the job ID is "MyPrinter-32".
	r := regexp.MustCompile(printerName + "-[0-9]+")

	if job = r.FindString(string(output)); job == "" {
		return "", errors.New("failed to find prompt for print job started")
	}
	return job, nil
}

// JobCompleted checks whether or not the given print job has been marked as
// completed.
func JobCompleted(ctx context.Context, printerName, job string) (bool, error) {
	out, err := testexec.CommandContext(ctx, "lpstat", "-W", "completed", "-o",
		printerName).Output(testexec.DumpLogOnError)
	if err != nil {
		return false, errors.Wrap(err, "failed to capture lpstat output")
	}

	return strings.Contains(string(out), job), nil
}
