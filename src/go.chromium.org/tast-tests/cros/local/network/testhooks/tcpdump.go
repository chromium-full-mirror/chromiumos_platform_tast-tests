// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

// tcpdumpHook implements hook interface.
type tcpdumpHook struct {
	noErrorHandlersMixin

	tcpdumpCmd *testexec.Cmd
}

func (h *tcpdumpHook) name() string {
	return "tcpdump"
}

func (h *tcpdumpHook) setUp(ctx context.Context) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get ContextOutDir")
	}

	logPath := filepath.Join(dir, "host_tcpdump.pcap")
	cmd := testexec.CommandContext(ctx,
		"tcpdump",
		"-i", "any",
		"-s100", // truncate the packet to reduce the size of the dump file
		"-U",    // write direct to file per each packet rather than buffered
		"-w", logPath)
	if err := cmd.Start(); err != nil {
		return err
	}

	h.tcpdumpCmd = cmd
	return nil
}

func (h *tcpdumpHook) tearDown(ctx context.Context, hasError func() bool) error {
	if h.tcpdumpCmd == nil {
		return nil
	}

	if err := h.tcpdumpCmd.Signal(unix.SIGTERM); err != nil {
		return errors.Wrap(err, "failed to send SIGKILL to the tcpdump process")
	}

	h.tcpdumpCmd.Wait()
	return nil
}
