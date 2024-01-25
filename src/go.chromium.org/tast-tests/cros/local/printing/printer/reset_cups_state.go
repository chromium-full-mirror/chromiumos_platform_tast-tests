// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package printer provides utilities about printer/cups.
package printer

import (
	"context"
	"time"

	ppb "go.chromium.org/chromiumos/system_api/printscanmgr_proto"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/debugd"
	"go.chromium.org/tast-tests/cros/local/printscanmgr"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
)

// checkDebugd makes sure debugd is running and responding to printer setup requests.  It does this
// by requesting a printer setup with an invalid PPD and making sure the returned error indicates a
// PPD problem rather than a debugd or d-bus problem.
func checkDebugd(ctx context.Context) error {
	// debugd should be very quick when things are working, so use a much shorter timeout.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.EnsureJobRunning(ctx, "debugd"); err != nil {
		return errors.Wrap(err, "failed to ensure debugd job is up")
	}
	d, err := debugd.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to debugd")
	}
	result, err := d.CupsAddManuallyConfiguredPrinter(ctx, "DebugdPrinterProbe", "", []byte(""))
	if err != nil {
		return errors.Wrap(err, "failed to call debugd.CupsAddManuallyConfiguredPrinter")
	} else if result != debugd.CUPSInvalidPPD {
		return errors.Wrapf(err, "unexpected response from debugd: got %s; want %s",
			result, debugd.CUPSInvalidPPD)
	}

	return nil
}

// checkPrintscanmgr performs the same check as `checkDebugd` above, but for
// printscanmgr instead of debugd.
func checkPrintscanmgr(ctx context.Context) error {
	// printscanmgr should be very quick when things are working, so use a much
	// shorter timeout.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.EnsureJobRunning(ctx, "printscanmgr"); err != nil {
		return errors.Wrap(err, "failed to ensure printscanmgr job is up")
	}
	p, err := printscanmgr.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to printscanmgr")
	}
	result, err := p.CupsAddManuallyConfiguredPrinter(ctx, &ppb.CupsAddManuallyConfiguredPrinterRequest{
				Name:        "DebugdPrinterProbe",
				Uri:         "",
				PpdContents: []byte("")})
	if err != nil {
		return errors.Wrap(err, "failed to call printscanmgr.CupsAddManuallyConfiguredPrinter")
	} else if result.Result != ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_INVALID_PPD {
		return errors.Wrapf(err, "unexpected response from printscanmgr: got %s; want %s",
			result.Result, ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_INVALID_PPD)
	}

	return nil
}

// ResetCups removes the privileged directories for cupsd. If cupsd is running,
// this stops it. If `usePrintscanmgr` is true, this will ensure printscanmgr is
// running. Otherwise, it will ensure debugd is running.
func ResetCups(ctx context.Context, usePrintscanmgr bool) error {
	// Make sure the appropriate daemon is running - users will need this to add a
	// printer after resetting CUPS.
	if (usePrintscanmgr) {
		if err := checkPrintscanmgr(ctx); err != nil {
			return errors.Wrap(err, "printscanmgr probe failed")
		}
	} else {
		if err := checkDebugd(ctx); err != nil {
			return errors.Wrap(err, "debugd probe failed")
		}
	}

	if err := upstart.StopJob(ctx, "cupsd"); err != nil {
		return err
	}
	if err := testexec.CommandContext(ctx, "systemd-tmpfiles", "--remove", "/usr/lib/tmpfiles.d/chromeos.conf").Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return upstart.StartJob(ctx, "cupsd")
}
