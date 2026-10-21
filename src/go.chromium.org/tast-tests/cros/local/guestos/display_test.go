// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/vm"
)

type fakeGuest struct {
	vm.Guest
	cmdFunc func(ctx context.Context, vshArgs ...string) *testexec.Cmd
}

func (g *fakeGuest) Command(ctx context.Context, vshArgs ...string) *testexec.Cmd {
	return g.cmdFunc(ctx, vshArgs...)
}

func TestIsX11Command(t *testing.T) {
	cases := []struct {
		name    string
		cmdline []string
		want    bool
	}{
		{
			name:    "X11CopyConfig",
			cmdline: append(X11CopyConfig.cmdArgs, "payload"),
			want:    true,
		},
		{
			name:    "X11PasteConfig",
			cmdline: X11PasteConfig.cmdArgs,
			want:    true,
		},
		{
			name:    "X11DemoConfig",
			cmdline: []string{X11DemoConfig().AppPath, "--width=100"},
			want:    true,
		},
		{
			name:    "WaylandCopyConfig",
			cmdline: append(WaylandCopyConfig.cmdArgs, "payload"),
			want:    false,
		},
		{
			name:    "WaylandPasteConfig",
			cmdline: WaylandPasteConfig.cmdArgs,
			want:    false,
		},
		{
			name:    "WaylandDemoConfig",
			cmdline: []string{WaylandDemoConfig().AppPath},
			want:    false,
		},
		{
			name:    "MousePerf",
			cmdline: []string{"python3", "mouse_perf.py"},
			want:    false,
		},
		{
			name:    "BareX11Demo",
			cmdline: []string{"x11_demo", "--width=100"},
			want:    true,
		},
		{
			name:    "NonMatchingX11DemoPrefix",
			cmdline: []string{"my_x11_demo"},
			want:    false,
		},
		{
			name:    "EmptyCmdline",
			cmdline: nil,
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isX11Command(tc.cmdline); got != tc.want {
				t.Errorf("isX11Command(%v) = %v, want %v", tc.cmdline, got, tc.want)
			}
		})
	}
}

func TestWaitForX11WindowFocusInvocationAndError(t *testing.T) {
	var capturedArgs []string
	guest := &fakeGuest{
		cmdFunc: func(ctx context.Context, vshArgs ...string) *testexec.Cmd {
			capturedArgs = append([]string(nil), vshArgs...)
			return testexec.CommandContext(ctx, "sh", "-c", "echo 'simulated focus timeout' >&2; exit 1")
		},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	err := waitForX11WindowFocus(ctx, guest, copyAppletTitle)
	if err == nil {
		t.Fatalf("waitForX11WindowFocus(ctx, guest, %q) = nil, want error", copyAppletTitle)
	}
	if !strings.Contains(err.Error(), "simulated focus timeout") {
		t.Errorf("waitForX11WindowFocus(ctx, guest, %q) error = %q, want substring %q", copyAppletTitle, err.Error(), "simulated focus timeout")
	}
	wantPrefix := []string{"python3", "-", copyAppletTitle}
	if len(capturedArgs) != 4 || !slices.Equal(capturedArgs[:3], wantPrefix) {
		t.Fatalf("waitForX11WindowFocus(ctx, guest, %q) invoked guest.Command with args %v, want prefix %v", copyAppletTitle, capturedArgs, wantPrefix)
	}
	// With a 2s context deadline (<= the 2s graceBuffer), waitForX11WindowFocus
	// halves the remaining time so the guest script timeout is in (0, 1.0].
	if timeoutSec, err := strconv.ParseFloat(capturedArgs[3], 64); err != nil || timeoutSec <= 0 || timeoutSec > 1.0 {
		t.Fatalf("waitForX11WindowFocus(ctx, guest, %q) timeout arg = %q, want float in (0, 1.0]", copyAppletTitle, capturedArgs[3])
	}

	// Verify the branch where the parent context deadline (5s) is between
	// graceBuffer (2s) and x11FocusTimeout+graceBuffer (12s): waitForX11WindowFocus
	// subtracts the 2s graceBuffer from the remaining ~5s deadline, passing a
	// timeout in (2.0, 3.0] seconds so the guest script exits before vsh is killed.
	midCtx, midCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer midCancel()
	if err := waitForX11WindowFocus(midCtx, guest, copyAppletTitle); err == nil {
		t.Fatalf("waitForX11WindowFocus(midCtx, guest, %q) = nil, want error", copyAppletTitle)
	}
	if len(capturedArgs) != 4 || !slices.Equal(capturedArgs[:3], wantPrefix) {
		t.Fatalf("waitForX11WindowFocus(midCtx, guest, %q) invoked guest.Command with args %v, want prefix %v", copyAppletTitle, capturedArgs, wantPrefix)
	}
	if timeoutSec, err := strconv.ParseFloat(capturedArgs[3], 64); err != nil || timeoutSec <= 2.0 || timeoutSec > 3.0 {
		t.Fatalf("waitForX11WindowFocus(midCtx, guest, %q) timeout arg = %q, want float in (2.0, 3.0]", copyAppletTitle, capturedArgs[3])
	}

	// Verify the default 10s timeout cap, stdin script delivery, and nil return on success.
	guest.cmdFunc = func(ctx context.Context, vshArgs ...string) *testexec.Cmd {
		capturedArgs = append([]string(nil), vshArgs...)
		return testexec.CommandContext(ctx, "python3", "-c", "import sys; data = sys.stdin.read(); sys.exit(0 if data == sys.argv[1] else 1)", x11FocusWaitScript)
	}
	if err := waitForX11WindowFocus(t.Context(), guest, copyAppletTitle); err != nil {
		t.Fatalf("waitForX11WindowFocus(t.Context(), guest, %q) = %v, want nil", copyAppletTitle, err)
	}
	wantArgs := []string{"python3", "-", copyAppletTitle, "10.00"}
	if !slices.Equal(capturedArgs, wantArgs) {
		t.Fatalf("waitForX11WindowFocus(t.Context(), guest, %q) args = %v, want %v", copyAppletTitle, capturedArgs, wantArgs)
	}

	// Verify the unwrapped error path when stderr is empty.
	guest.cmdFunc = func(ctx context.Context, vshArgs ...string) *testexec.Cmd {
		return testexec.CommandContext(ctx, "false")
	}
	if err := waitForX11WindowFocus(t.Context(), guest, copyAppletTitle); err == nil {
		t.Fatalf("waitForX11WindowFocus(t.Context(), guest, %q) = nil on empty-stderr failure, want error", copyAppletTitle)
	} else if strings.Contains(err.Error(), "X11 focus check failed") {
		t.Errorf("waitForX11WindowFocus(t.Context(), guest, %q) error = %q, want unwrapped error without 'X11 focus check failed'", copyAppletTitle, err.Error())
	}
}

func TestWaitForX11WindowFocusScript(t *testing.T) {
	for _, mode := range []string{"async_wm_focus", "fallback_set_focus", "timeout_unmapped"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			cmd := testexec.CommandContext(
				ctx, "python3", "-B", "x11_focus_wait_test.py",
				"X11FocusWaitTest.test_"+mode,
			)
			if out, err := cmd.CombinedOutput(testexec.DumpLogOnError); err != nil {
				t.Fatalf("x11_focus_wait_test.py (%s) failed: %v (output: %s)", mode, err, strings.TrimSpace(string(out)))
			}
		})
	}
}
