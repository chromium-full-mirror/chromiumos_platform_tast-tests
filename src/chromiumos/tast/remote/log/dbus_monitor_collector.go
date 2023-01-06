// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package log

import (
	"context"
	"io"

	"chromiumos/tast/errors"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
)

// DBusMonitorCollector runs and collects the output of dbus-monitor.
type DBusMonitorCollector struct {
	host *ssh.Conn
	buf  Buffer
	cmd  *ssh.Cmd
}

// StartDBusMonitorCollector starts a new DBusMonitorCollector on the host.
func StartDBusMonitorCollector(ctx context.Context, host *ssh.Conn, dbusMonitorArgs ...string) (*DBusMonitorCollector, error) {
	c := &DBusMonitorCollector{
		host: host,
	}
	if err := c.start(ctx, dbusMonitorArgs); err != nil {
		return nil, err
	}
	return c, nil
}

// start spawns the dbus-monitor process to begin collecting D-Bus messages on
// the host as they are reported.
func (c *DBusMonitorCollector) start(ctx context.Context, dbusMonitorArgs []string) error {
	testing.ContextLog(ctx, "Starting dbus-monitor with args ", dbusMonitorArgs)
	cmd := c.host.CommandContext(ctx, "dbus-monitor", dbusMonitorArgs...)
	cmd.Stdout = &c.buf
	cmd.Stderr = &c.buf
	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "failed to run dbus-monitor with args %v", dbusMonitorArgs)
	}
	c.cmd = cmd
	return nil
}

// Dump copies the contents collected to w and resets the buffer.
func (c *DBusMonitorCollector) Dump(w io.Writer) error {
	return c.buf.Dump(w)
}

// Reset resets buffer, clearing any previously collected messages since the
// start of log collection or the last Dump call.
func (c *DBusMonitorCollector) Reset() {
	c.buf.Reset()
}

// Close stops the collector and resets the message buffer.
func (c *DBusMonitorCollector) Close() error {
	c.cmd.Abort()
	// Ignore the error as wait always has error on aborted command.
	_ = c.cmd.Wait()
	c.buf.Reset()
	return nil
}
