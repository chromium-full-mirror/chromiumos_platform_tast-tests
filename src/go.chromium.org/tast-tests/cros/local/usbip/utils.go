// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"context"
	"os"
	"path"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/tcpdump"
	"go.chromium.org/tast/core/errors"
)

type cleanupFn func(ctx context.Context) error

// StartLocalTCPDump starts local TCP tracing. This could be useful for debugging.
// Returns a cleanup function that stops TCP tracing.
func StartLocalTCPDump(ctx context.Context, outDir string) (cleanupFn, error) {
	tcpdump := tcpdump.NewLocalRunner()
	iface := "lo"
	pcapPath := path.Join(outDir, "tcpdump.pcap")
	stdoutPath := path.Join(outDir, "tcpdump.stdout")
	stderrPath := path.Join(outDir, "tcpdump.stderr")

	stdoutFile, err := prepareDirFile(ctx, stdoutPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open stdout log of tcpdump")
	}
	stderrFile, err := prepareDirFile(ctx, stderrPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open stderr log of tcpdump")
	}
	if err := tcpdump.StartTcpdump(ctx, iface, pcapPath, stdoutFile, stderrFile); err != nil {
		return nil, errors.Wrap(err, "failed to start tcpdump")
	}

	return func(ctx context.Context) error {
		if err := tcpdump.Close(ctx); err != nil {
			return errors.Wrap(err, "failed to close tcpdump")
		}
		return nil
	}, nil
}

// AddUSBIPModules adds usbip kernel modules required for USBIP server to work.
// Returns a cleanup function that removes added modules.
func AddUSBIPModules(ctx context.Context) (cleanupFn, error) {
	cmd := testexec.CommandContext(ctx, "modprobe", "-a", "usbip_core", "vhci-hcd")
	if err := cmd.Run(); err != nil {
		return nil, errors.Wrap(err, "failed to install usbip kernel modules")
	}

	return func(ctx context.Context) error {
		cmd := testexec.CommandContext(ctx, "modprobe", "-r", "-a", "vhci-hcd", "usbip_core")
		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "failed to remove usbip kernel modules")
		}
		return nil
	}, nil
}

// prepareDirFile prepares the base directory for filename and opens the file.
func prepareDirFile(ctx context.Context, filename string) (*os.File, error) {
	if err := os.MkdirAll(path.Dir(filename), 0755); err != nil {
		return nil, errors.Wrapf(err, "failed to create basedir for %q", filename)
	}
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file %q", filename)
	}
	return f, nil
}
