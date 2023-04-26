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

// TailCollector watches a file on remote host and collects the appended
// contents.
type TailCollector struct {
	host                    *ssh.Conn
	buf                     Buffer
	path                    string
	cmd                     *ssh.Cmd
	tailFollowNameSupported bool
}

// StartTailCollector spawns a log collector on file p on host that relies on
// the "tail" command to collect logs.
//
// Set tailFollowNameSupported to true if the host's tail implementation
// supports the "--follow=name" command to allow for tail following to stay
// at the same filename rather than follow the file if the name changes.
func StartTailCollector(ctx context.Context, host *ssh.Conn, p string, tailFollowNameSupported bool) (*TailCollector, error) {
	c := &TailCollector{
		host:                    host,
		path:                    p,
		tailFollowNameSupported: tailFollowNameSupported,
	}
	if err := c.start(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// start spawns the tail command to track the target log file.
func (c *TailCollector) start(ctx context.Context) error {
	var followArg string
	if c.tailFollowNameSupported {
		followArg = "--follow=name"
	} else {
		followArg = "-f"
	}

	cmd := c.host.CommandContext(ctx, "tail", followArg, c.path)

	cmd.Stdout = &c.buf

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to run tail command")
	}
	c.cmd = cmd
	return nil
}

// Dump copies the contents collected to w and resets the log buffer.
func (c *TailCollector) Dump(w io.Writer) error {
	return c.buf.Dump(w)
}

// Reset resets log buffer, clearing any previously collected logs since the
// start of log collection or the last Dump call.
func (c *TailCollector) Reset() {
	c.buf.Reset()
}

// Close stops the collector and resets the log buffer.
func (c *TailCollector) Close() error {
	c.cmd.Abort()
	// Ignore the error as wait always has error on aborted command.
	_ = c.cmd.Wait()
	c.Reset()
	return nil
}
