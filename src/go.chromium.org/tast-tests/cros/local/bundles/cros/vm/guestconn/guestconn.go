// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package guestconn provides the util functions needed for host guest communication.
package guestconn

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"golang.org/x/sys/unix"
)

const (
	// LibFile is the file name of the guest python library file.
	LibFile = "guestlib.py"

	// PrefixReady should be received from guest when guest is set up
	PrefixReady = "READY:"

	// PrefixReadyNoDrop should be received from guest when guest is set up but not dropping cache:
	PrefixReadyNoDrop = "READY_NO_DROP:"

	// Run should be sent to guest when guest should run a task
	Run = "RUN\n"

	// End will be received from guest when guest finished one task
	End = "END"

	// Complete will be received from guest when guest finished all tasks
	Complete = "COMPLETE"
)

// CreateGuestConn creates a guest connection with a pair of FIFO files.
func CreateGuestConn(dirPath string, ps *vm.CrosvmParams) (toGuestFIFO, fromGuestFIFO string, err error) {
	// Use FIFO files as the guest's serial device.
	toGuestFIFO = filepath.Join(dirPath, "input.fifo")
	if err := unix.Mkfifo(toGuestFIFO, 0666); err != nil {
		return "", "", errors.Wrap(err, "failed to make input fifo")
	}
	fromGuestFIFO = filepath.Join(dirPath, "output.fifo")
	if err := unix.Mkfifo(fromGuestFIFO, 0666); err != nil {
		return "", "", errors.Wrap(err, "failed to make output fifo")
	}
	vm.SerialIO(toGuestFIFO, fromGuestFIFO, vm.OtherSerial)(ps)

	return toGuestFIFO, fromGuestFIFO, nil
}

// OpenGuestConn opens the guest connection.
func OpenGuestConn(ctx context.Context, toGuestPath, fromGuestPath string) (toGuest, fromGuest *os.File, cleanUp func(ctx context.Context), err error) {
	toGuest, err = os.OpenFile(toGuestPath, os.O_WRONLY, 0755)
	if err != nil {
		return nil, nil, cleanUp, errors.Wrap(err, "failed to make input fifo")
	}
	cleanUp = func(ctx context.Context) {
		toGuest.Close()
	}

	// Prepare the deadline for the FIFO files.
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, nil, cleanUp, errors.Wrap(err, "no deadline is set to the context")
	}
	if err = toGuest.SetDeadline(deadline); err != nil {
		return nil, nil, cleanUp, errors.Wrap(err, "failed to set deadline for FIFO file to the guest")
	}

	fromGuest, err = os.Open(fromGuestPath)
	if err != nil {
		return nil, nil, cleanUp, errors.Wrap(err, "failed to open guest output")
	}
	cleanUp = func(ctx context.Context) {
		toGuest.Close()
		fromGuest.Close()
	}

	if err = fromGuest.SetDeadline(deadline); err != nil {
		return nil, nil, cleanUp, errors.Wrap(err, "failed to set deadline for FIFO file to the guest")
	}
	return toGuest, fromGuest, cleanUp, nil
}

// WaitForPrefix reads line from reader until the line starts with one of
// items in expectedPrefixes.
func WaitForPrefix(reader *bufio.Reader, prefixes []string) (string, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", errors.Wrap(err, "failed to read line")
		}

		for _, pre := range prefixes {
			if strings.HasPrefix(line, pre) {
				return line, nil
			}
		}
	}
}
