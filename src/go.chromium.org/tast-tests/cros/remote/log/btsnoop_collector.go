// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package log

import (
	"context"
	"io"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
)

// BtsnoopCollector collects btsnoop log with btmon command.
type BtsnoopCollector struct {
	host *ssh.Conn
	buf  Buffer
	cmd  *ssh.Cmd
}

// StartBtsnoopCollector spawns a log collector on the host that relies on
// the "logread" command to collect logs.
func StartBtsnoopCollector(ctx context.Context, host *ssh.Conn) (*BtsnoopCollector, error) {
	c := &BtsnoopCollector{
		host: host,
	}
	if err := c.start(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// start spawns a btmon process to begin reading btsnoop logs.
func (c *BtsnoopCollector) start(ctx context.Context) error {
	// Start collecting new logs as they are reported.
	cmd := c.host.CommandContext(ctx, "btmon", "-t")
	if cmd == nil {
		return errors.New("failed to create a cmd")
	}
	cmd.Stdout = &c.buf
	cmd.Stderr = &c.buf
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start btmon")
	}
	// Keep a reference to the running cmd so that it can be killed when Close
	// is called.
	c.cmd = cmd
	return nil
}

// Dump copies the contents collected to w and resets the log buffer.
func (c *BtsnoopCollector) Dump(w io.Writer) error {
	return c.buf.Dump(w)
}

// Reset resets the log buffer, clearing any previously collected logs since the
// start of log collection or the last Dump call.
func (c *BtsnoopCollector) Reset() {
	c.buf.Reset()
}

// Close stops the collector and resets the log buffer.
func (c *BtsnoopCollector) Close() error {
	defer c.Reset()
	if c.cmd == nil {
		return nil
	}
	c.cmd.Abort()
	// Ignore the error as wait always has error on aborted command.
	_ = c.cmd.Wait()
	return nil
}
