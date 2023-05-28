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

// JournalctlCollector collects messages from a remote host using the journalctl
// command.
type JournalctlCollector struct {
	host                *ssh.Conn
	buf                 Buffer
	cmd                 *ssh.Cmd
	extraJournalctlArgs []string
}

// StartJournalctlCollector spawns a log collector on the host that relies on
// the "journalctl" command to collect logs.
func StartJournalctlCollector(ctx context.Context, host *ssh.Conn, extraJournalctlArgs ...string) (*JournalctlCollector, error) {
	c := &JournalctlCollector{
		host:                host,
		extraJournalctlArgs: extraJournalctlArgs,
	}
	if err := c.start(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// start spawns the journalctl process to begin reading syslogd logs as they are
// reported.
func (c *JournalctlCollector) start(ctx context.Context) error {
	// Start collecting new logs as they are reported.
	args := []string{"-f"}
	args = append(args, c.extraJournalctlArgs...)
	cmd := c.host.CommandContext(ctx, "journalctl", args...)
	cmd.Stdout = &c.buf
	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "failed to start journalctl with args %v", args)
	}
	// Keep a reference to the running cmd so that it can be killed when Close
	// is called.
	c.cmd = cmd
	return nil
}

// Dump copies the contents collected to w and resets the log buffer.
func (c *JournalctlCollector) Dump(w io.Writer) error {
	return c.buf.Dump(w)
}

// Reset resets log buffer, clearing any previously collected logs since the
// start of log collection or the last Dump call.
func (c *JournalctlCollector) Reset() {
	c.buf.Reset()
}

// Close stops the collector and resets the log buffer.
func (c *JournalctlCollector) Close() error {
	c.cmd.Abort()
	// Ignore the error as wait always has error on aborted command.
	_ = c.cmd.Wait()
	c.Reset()
	return nil
}
