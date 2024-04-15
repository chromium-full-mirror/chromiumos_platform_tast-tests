// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// processes returns all (likely two) the forked processes of mitmdump.
func processes() ([]*process.Process, error) {
	return procutil.FindAll(func(p *process.Process) bool {
		exe, err := p.Exe()
		return err == nil && (strings.HasSuffix(exe, "mitmdump"))
	})
}

// forceKill attempts to forcibly terminate all the forked mitmdump processes if they are running. This should be a last resort to clean up running processes.
func forceKill(ctx context.Context) error {
	procs, err := processes()
	// ErrNotFound is returned when no proc is found.
	if err == procutil.ErrNotFound {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to get mitmproxy processes")
	}

	testing.ContextLog(ctx, "mitmproxy: force killing procs: ", len(procs))
	for _, proc := range procs {
		if err := proc.Kill(); err != nil { // SIGKILL
			return errors.Wrapf(err, "failed to force kill %s", proc.String())
		}
	}
	return waitForProcTerminated(ctx, 10*time.Second)
}

// killCmd stops an actual proxy server process from a pidfile first, then all child processes and the parent `cmd` at the end. It waits until all of them finish.
func killCmd(ctx context.Context, cmd *testexec.Cmd, pid int) error {
	var errs []error
	if err := testexec.CommandContext(ctx, "kill", fmt.Sprintf("%d", pid)).Run(); err != nil {
		errs = append(errs, errors.Wrapf(err, "failed to kill proxy process from pid: %v", pid))
	}

	// Wait until all child processes are stopped. If not stopped, force kill them once more.
	if err := waitForProcTerminated(ctx, 10*time.Second); err != nil {
		if err := forceKill(ctx); err != nil {
			errs = append(errs, errors.New("failed to force kill all processes"))
		}
	}

	// Kill the parent process and ensure that it has stopped.
	if err := cmd.Kill(); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to kill parent process"))
	}
	if err := cmd.Wait(); err != nil {
		errs = append(errs, errors.Wrap(err, "parent process has not stopped"))
	}
	return errors.Join(errs...)
}

// waitForProcTerminated waits for all forked proxy processes to exit.
func waitForProcTerminated(ctx context.Context, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		procs, err := processes()
		if err == procutil.ErrNotFound {
			return nil
		}
		for _, proc := range procs {
			r, err := proc.IsRunning()
			if err == nil && r {
				return errors.Errorf("process %d is still running", proc.Pid)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}
