// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package swap provides set of util functions for crosvm vmm-swap.
package swap

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v3/process"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/procutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Status represents the current status of crosvm vmm-swap.
type Status int

// Status of crosvm vmm-swap.
const (
	None Status = iota
	Ready
	Pending
	TrimInProgress
	SwapOutInProgress
	SwapInInProgress
	Active
	Failed
)

const (
	daemonStoreBase = "/run/daemon-store/crosvm"
)

var (
	errCrosvmNotFound = errors.New("crosvm process not found")
	errEnableSwap     = errors.New("crosvm failed to enable vmm-swap")
	errSwapInProgress = errors.New("crosvm swap-out in progress")
)

// ARCVMSocketPath look-ups a crosvm socket path to communicate with ARCVM
func ARCVMSocketPath() (string, error) {
	socketPath := ""
	var pathMismatchErr error
	_, err := procutil.FindAll(procutil.And(procutil.ByExe("/usr/bin/crosvm"), func(p *process.Process) bool {
		cmdline, err := p.CmdlineSlice()
		if err != nil {
			return false
		}
		for i, arg := range cmdline {
			// Although there are multiple ways to specify the
			// control socket, ARCVM currently always uses `--socket
			// PATH`. We could try to be more resiliant against
			// future changes, but parsing the cmdline like this
			// isn't a stable API to begin with. So just hardcode
			// things to handle the current behavior, and the test
			// can be updated if ARCVM changes.
			if arg == "--socket" && i+1 < len(cmdline) {
				socketArg := cmdline[i+1]
				if filepath.Base(socketArg) == "arcvm.sock" {
					if socketPath != "" && socketPath != socketArg {
						pathMismatchErr = errors.New("too many ARCVM sockets")
					} else {
						socketPath = socketArg
					}
					return true
				}
			}
		}
		return false
	}))
	if err != nil {
		return "", errors.Wrap(err, "failed to find ARCVM process")
	}
	return socketPath, pathMismatchErr
}

// CurrentStatus fetches the current swap status from a running crosvm instance.
// socketPath should be the absolute path of a crosvm control socket.
func CurrentStatus(ctx context.Context, socketPath string) (Status, error) {
	dump, err := testexec.CommandContext(ctx, "crosvm", "swap", "status", socketPath).Output(testexec.DumpLogOnError)
	if err != nil {
		return None, errors.Wrap(err, "swap status")
	}
	status := string(dump)
	for _, item := range []struct {
		text   string
		status Status
	}{
		{"Ready", Ready},
		{"Pending", Pending},
		{"TrimInProgress", TrimInProgress},
		{"SwapOutInProgress", SwapOutInProgress},
		{"SwapInInProgress", SwapInInProgress},
		{"Active", Active},
		{"Failed", Failed},
	} {
		if strings.Contains(status, item.text) {
			return item.status, nil
		}
	}
	return None, errors.Errorf("unexpected swap status: %s", status)
}

// WaitForStatus polls the swap status until swap reaches one of the goal states.
func WaitForStatus(ctx context.Context, socketPath string, goals []Status) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		status, err := CurrentStatus(ctx, socketPath)
		if err != nil {
			return testing.PollBreak(err)
		}
		if status == Failed {
			return testing.PollBreak(errEnableSwap)
		}

		for _, goal := range goals {
			if status == goal {
				return nil
			}
		}

		return errors.Errorf("non-target status %v", status)
	}, nil)
}

// Enable enables vmm-swap of crosvm.
func Enable(ctx context.Context, socketPath string) error {
	if err := testexec.CommandContext(ctx, "crosvm", "swap", "enable", socketPath).Run(); err != nil {
		return errors.Wrap(err, "enable vmm-swap")
	}
	return nil
}

// StartSwapOut starts swapping out all the guest memory.
func StartSwapOut(ctx context.Context, socketPath string) error {
	if err := testexec.CommandContext(ctx, "crosvm", "swap", "out", socketPath).Run(); err != nil {
		return errors.Wrap(err, "vmm-swap out")
	}
	return nil
}

// Disable disables vmm-swap of crosvm and swap in all the guest memory.
func Disable(ctx context.Context, socketPath string) error {
	if err := testexec.CommandContext(ctx, "crosvm", "swap", "disable", socketPath).Run(); err != nil {
		return errors.Wrap(err, "enable vmm-swap")
	}
	return nil
}
