// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package nebraska provides start/stop functions for Nebraska.
package nebraska

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// Nebraska struct hold Nebraska server runtime information.
type Nebraska struct {
	cmd  *testexec.Cmd
	Port int
}

// Start starts the Nebraska server and returns the Nebraska struct on a
// successful bringup, otherwise an error is returned.
func Start(ctx context.Context, root string, args []string) (*Nebraska, error) {
	args = append([]string{"--runtime-root", root}, args...)
	cmd := testexec.CommandContext(ctx, "nebraska.py", args...)
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start Nebraska")
	}

	success := false
	defer func() {
		if success {
			return
		}
		cmd.Kill()
		cmd.Wait()
	}()

	portPath := filepath.Join(root, "port")

	// Try a few times to make sure Nebraska is up.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(portPath); err != nil {
			if os.IsNotExist(err) {
				return err
			}
			return testing.PollBreak(err)
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Second * 5}); err != nil {
		return nil, errors.Wrap(err, "Nebraska did not start")
	}

	portStr, err := ioutil.ReadFile(portPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the Nebraska's port file")
	}

	port, err := strconv.Atoi(string(portStr))
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse the Nebraska's port file")
	}

	success = true
	return &Nebraska{
		Port: port,
		cmd:  cmd,
	}, nil
}

// Stop stops the Nebraska.
func (n *Nebraska) Stop(ctx context.Context) error {
	// Kill the Nebraska instance with SIGINT so it has time to remove port/pid files
	// and cleanup properly.
	n.cmd.Signal(unix.SIGINT)
	ok := false

	errc := make(chan error)
	go func() {
		errc <- n.cmd.Wait(testexec.DumpLogOnError)
	}()

	select {
	case err := <-errc:
		if err == nil {
			ok = true
		}
	case <-ctx.Done():
	case <-time.After(15 * time.Second):
	}

	if !ok {
		return errors.New("failed to wait until the Nebraska process stopped")
	}

	return nil
}
