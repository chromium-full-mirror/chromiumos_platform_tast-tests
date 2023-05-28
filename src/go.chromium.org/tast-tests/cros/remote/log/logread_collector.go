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

// LogreadCollector collects messages in syslogd's circular log Buffer using
// the logread command.
type LogreadCollector struct {
	host *ssh.Conn
	buf  Buffer
	cmd  *ssh.Cmd
}

// StartLogreadCollector spawns a log collector on the host that relies on
// the "logread" command to collect logs.
func StartLogreadCollector(ctx context.Context, host *ssh.Conn) (*LogreadCollector, error) {
	c := &LogreadCollector{
		host: host,
	}
	if err := c.start(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// start spawns the logread process to begin reading syslogd logs as they are
// reported, as well as collect the existing logs in the syslogd circular log
// buffer.
func (c *LogreadCollector) start(ctx context.Context) error {
	// Collect existing logs in syslogd Buffer.
	cmd := c.host.CommandContext(ctx, "logread")
	cmd.Stdout = &c.buf
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to run logread")
	}
	// Start collecting new logs as they are reported.
	cmd = c.host.CommandContext(ctx, "logread", "-f")
	cmd.Stdout = &c.buf
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start logread -f")
	}
	// Keep a reference to the running cmd so that it can be killed when Close
	// is called.
	c.cmd = cmd
	return nil
}

// Dump copies the contents collected to w and resets the log buffer.
func (c *LogreadCollector) Dump(w io.Writer) error {
	return c.buf.Dump(w)
}

// Reset resets log buffer, clearing any previously collected logs since the
// start of log collection or the last Dump call.
func (c *LogreadCollector) Reset() {
	c.buf.Reset()
}

// Close stops the collector and resets the log buffer.
func (c *LogreadCollector) Close() error {
	c.cmd.Abort()
	// Ignore the error as wait always has error on aborted command.
	_ = c.cmd.Wait()
	c.Reset()
	return nil
}
