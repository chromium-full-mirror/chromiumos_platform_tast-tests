// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package swap provides set of util functions for crosvm vmm-swap.
package swap

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// TestApk is the apk used by swap tests
	TestApk = "ArcSwapTest.apk"
	// AppName is the name of the swap test app
	AppName = "ARC Swap Test"
	// Pkg is the package name of the swap test app
	Pkg = "org.chromium.arc.testapp.swap"
	// Activity is the activity name of the swap test activity
	Activity = ".SwapActivity"
	// ReceiverAction is the intent action for displaying a notification
	ReceiverAction = "org.chromium.arc.testapp.swap.NOTIFICATION"
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
	// UnrestrictedTimeout is the maximum amount of time that swapping out is
	// expected to take when ARC is running with chrome.UnRestrictARCCPU().
	UnrestrictedTimeout = 60 * time.Second
	daemonStoreBase     = "/run/daemon-store/crosvm"
)

var (
	errCrosvmNotFound = errors.New("crosvm process not found")
	errEnableSwap     = errors.New("crosvm failed to enable vmm-swap")
	errSwapInProgress = errors.New("crosvm swap-out in progress")
)

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

// ForceEnableSwap immediately swaps ARCVM memory to disk with vmm-swap,
// ignoring any disk write limits.
func ForceEnableSwap(ctx context.Context) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(cleanupCtx)

	// Use Chrome's force enable vmm-swap keyboard shortcut to immediately
	// trigger swap and bypass disk write limits. This shortcut is used for
	// demoing and testing vmm-swap and may go away when vmm-swap is more
	// mature, but we can use it for now.
	err = keyboard.Accel(ctx, "Ctrl+Alt+Shift+O")
	if err != nil {
		return errors.Wrap(err, "failed to inject force swap keys")
	}

	return nil
}
