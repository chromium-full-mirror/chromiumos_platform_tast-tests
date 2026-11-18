// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"bytes"
	"context"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
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

func newTestPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	return r, w
}

func waitForPipeByte(t *testing.T, r, w *os.File) {
	t.Helper()
	w.Close()
	var b [1]byte
	if _, err := r.Read(b[:]); err != nil {
		t.Fatalf("failed waiting for guest command readiness: %v", err)
	}
}

// fakeGuestProcessCmd spawns TestFakeGuestHelperProcess in a child test binary
// so SIGTERM delivery uses Go's signal runtime without shell trap/read races.
func fakeGuestProcessCmd(ctx context.Context, mode, stdoutText, stderrText string, readyW *os.File) *testexec.Cmd {
	cmd := testexec.CommandContext(ctx, os.Args[0], "-test.run=^TestFakeGuestHelperProcess$", "--", mode, stdoutText, stderrText)
	cmd.Env = append(os.Environ(), "GO_WANT_FAKE_GUEST_HELPER=1")
	cmd.ExtraFiles = []*os.File{readyW}
	return cmd
}

func TestFakeGuestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_FAKE_GUEST_HELPER") != "1" {
		return
	}
	idx := slices.Index(os.Args, "--")
	if idx < 0 || len(os.Args[idx+1:]) != 3 {
		os.Exit(2)
	}
	args := os.Args[idx+1:]
	mode, stdoutText, stderrText := args[0], args[1], args[2]

	var sigCh chan os.Signal
	switch mode {
	case "exit-on-term":
		sigCh = make(chan os.Signal, 1)
		signal.Notify(sigCh, unix.SIGTERM)
	case "ignore-term":
		signal.Ignore(unix.SIGTERM)
	case "block":
	default:
		os.Exit(2)
	}
	if stdoutText != "" {
		_, _ = os.Stdout.WriteString(stdoutText)
	}
	if stderrText != "" {
		_, _ = os.Stderr.WriteString(stderrText)
	}
	readyW := os.NewFile(3, "ready")
	_, _ = readyW.Write([]byte("r"))
	_ = readyW.Close()

	if sigCh != nil {
		<-sigCh
		os.Exit(0)
	}
	blockR, blockW, err := os.Pipe()
	if err != nil {
		os.Exit(2)
	}
	defer blockW.Close()
	_, _ = blockR.Read(make([]byte, 1))
}

func TestLaunchWindowedAppSingleAttempt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cmdline []string
		timeout time.Duration
	}{
		{
			name:    "WaylandLongTimeout",
			cmdline: WaylandCopyConfig.cmdArgs,
			timeout: 120 * time.Second,
		},
		{
			name:    "X11ShortTimeoutBoundary",
			cmdline: X11CopyConfig.cmdArgs,
			timeout: 30 * time.Second,
		},
		{
			name:    "X11BelowTwoAttemptThreshold",
			cmdline: X11CopyConfig.cmdArgs,
			timeout: 45 * time.Second,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			var pollTimeouts []time.Duration
			guest := &fakeGuest{
				cmdFunc: func(ctx context.Context, vshArgs ...string) *testexec.Cmd {
					attempts++
					return testexec.CommandContext(ctx, "sh", "-c", "echo guest-err >&2; exit 1")
				},
			}
			var stdoutBuf, stderrBuf bytes.Buffer
			_, _, _, err := launchWindowedApp(t.Context(), guest, tc.timeout, copyAppletTitle, tc.cmdline, &stdoutBuf, &stderrBuf, func(ctx context.Context, pollTimeout time.Duration) (coords.Size, error) {
				pollTimeouts = append(pollTimeouts, pollTimeout)
				return coords.Size{}, errors.New("window missing")
			})
			if err == nil || !strings.Contains(err.Error(), "failed to find window") {
				t.Fatalf("launchWindowedApp(..., %v, %v) error = %v, want substring 'failed to find window'", tc.timeout, tc.cmdline, err)
			}
			if attempts != 1 {
				t.Errorf("launchWindowedApp(..., %v, %v) attempts = %d, want 1", tc.timeout, tc.cmdline, attempts)
			}
			if !slices.Equal(pollTimeouts, []time.Duration{tc.timeout}) {
				t.Errorf("launchWindowedApp(..., %v, %v) pollTimeouts = %v, want [%v]", tc.timeout, tc.cmdline, pollTimeouts, tc.timeout)
			}
		})
	}
}

func TestLaunchWindowedAppX11RelaunchRecovery(t *testing.T) {
	ready1R, ready1W := newTestPipe(t)
	ready2R, ready2W := newTestPipe(t)

	attempts := 0
	var attempt1Cmd *testexec.Cmd
	var pollTimeouts []time.Duration
	guest := &fakeGuest{
		cmdFunc: func(ctx context.Context, vshArgs ...string) *testexec.Cmd {
			attempts++
			if attempts == 1 {
				cmd := fakeGuestProcessCmd(ctx, "exit-on-term", "stale-out", "stale-err", ready1W)
				attempt1Cmd = cmd
				return cmd
			}
			if attempt1Cmd == nil || attempt1Cmd.ProcessState == nil || attempt1Cmd.ProcessState.ExitCode() != 0 {
				t.Errorf("launchWindowedApp(..., 120s, %v) attempt 1 ProcessState before attempt 2 = %v, want reaped after SIGTERM (exit 0)", X11CopyConfig.cmdArgs, attempt1Cmd.ProcessState)
			}
			return fakeGuestProcessCmd(ctx, "exit-on-term", "recovered-out", "", ready2W)
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	wantSize := coords.NewSize(200, 200)
	cmd, cmdCancel, gotSize, err := launchWindowedApp(t.Context(), guest, 120*time.Second, copyAppletTitle, X11CopyConfig.cmdArgs, &stdoutBuf, &stderrBuf, func(ctx context.Context, pollTimeout time.Duration) (coords.Size, error) {
		pollTimeouts = append(pollTimeouts, pollTimeout)
		if len(pollTimeouts) == 1 {
			waitForPipeByte(t, ready1R, ready1W)
			return coords.Size{}, errors.New("attempt 1 unmapped")
		}
		waitForPipeByte(t, ready2R, ready2W)
		return wantSize, nil
	})
	if err != nil {
		t.Fatalf("launchWindowedApp(..., 120s, %v) = %v, want nil", X11CopyConfig.cmdArgs, err)
	}
	if cmd.ProcessState != nil {
		t.Errorf("launchWindowedApp(..., 120s, %v) attempt 2 exited prematurely before cleanup", X11CopyConfig.cmdArgs)
	}
	if !terminateGuestCommand(t.Context(), cmd, cmdCancel, X11CopyConfig.cmdArgs) {
		t.Errorf("terminateGuestCommand() = false, want true")
	}
	if attempts != 2 {
		t.Errorf("launchWindowedApp(..., 120s, %v) attempts = %d, want 2", X11CopyConfig.cmdArgs, attempts)
	}
	wantPolls := []time.Duration{25 * time.Second, 25 * time.Second}
	if !slices.Equal(pollTimeouts, wantPolls) {
		t.Errorf("launchWindowedApp(..., 120s, %v) pollTimeouts = %v, want %v", X11CopyConfig.cmdArgs, pollTimeouts, wantPolls)
	}
	if gotSize != wantSize {
		t.Errorf("launchWindowedApp(..., 120s, %v) size = %v, want %v", X11CopyConfig.cmdArgs, gotSize, wantSize)
	}
	if got := stdoutBuf.String(); got != "recovered-out" {
		t.Errorf("launchWindowedApp(..., 120s, %v) stdout = %q, want %q", X11CopyConfig.cmdArgs, got, "recovered-out")
	}
	if got := stderrBuf.String(); got != "" {
		t.Errorf("launchWindowedApp(..., 120s, %v) stderr = %q, want empty", X11CopyConfig.cmdArgs, got)
	}
}

func TestLaunchWindowedAppUncleanStop(t *testing.T) {
	readyReader, readyWriter := newTestPipe(t)

	attempts := 0
	var attempt1Cmd *testexec.Cmd
	guest := &fakeGuest{
		cmdFunc: func(cmdCtx context.Context, vshArgs ...string) *testexec.Cmd {
			attempts++
			cmd := fakeGuestProcessCmd(cmdCtx, "ignore-term", "", "unclean-stderr", readyWriter)
			attempt1Cmd = cmd
			return cmd
		},
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	_, _, _, err := launchWindowedApp(t.Context(), guest, 120*time.Second, copyAppletTitle, X11CopyConfig.cmdArgs, &stdoutBuf, &stderrBuf, func(pollCtx context.Context, pollTimeout time.Duration) (coords.Size, error) {
		waitForPipeByte(t, readyReader, readyWriter)
		return coords.Size{}, errors.New("attempt 1 unmapped")
	})
	if err == nil || !strings.Contains(err.Error(), "failed to find window") {
		t.Fatalf("launchWindowedApp(..., 120s, %v) error = %v, want substring 'failed to find window'", X11CopyConfig.cmdArgs, err)
	}
	if attempts != 1 {
		t.Errorf("launchWindowedApp(..., 120s, %v) attempts = %d, want 1 when SIGTERM is ignored", X11CopyConfig.cmdArgs, attempts)
	}
	if attempt1Cmd == nil || attempt1Cmd.ProcessState == nil {
		t.Fatalf("launchWindowedApp(..., 120s, %v) did not reap attempt 1 process", X11CopyConfig.cmdArgs)
	}
	if attempt1Cmd.ProcessState.ExitCode() == 0 {
		t.Errorf("launchWindowedApp(..., 120s, %v) attempt 1 ExitCode() = 0, want non-zero after SIGKILL fallback", X11CopyConfig.cmdArgs)
	}
	if got := stderrBuf.String(); got != "unclean-stderr" {
		t.Errorf("launchWindowedApp(..., 120s, %v) stderr = %q, want %q", X11CopyConfig.cmdArgs, got, "unclean-stderr")
	}
}

func TestLaunchWindowedAppExpiredContext(t *testing.T) {
	readyReader, readyWriter := newTestPipe(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	attempts := 0
	var attempt1Cmd *testexec.Cmd
	guest := &fakeGuest{
		cmdFunc: func(cmdCtx context.Context, vshArgs ...string) *testexec.Cmd {
			attempts++
			cmd := fakeGuestProcessCmd(cmdCtx, "block", "", "attempt1-stderr", readyWriter)
			attempt1Cmd = cmd
			return cmd
		},
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	_, _, _, err := launchWindowedApp(ctx, guest, 60*time.Second, copyAppletTitle, X11CopyConfig.cmdArgs, &stdoutBuf, &stderrBuf, func(pollCtx context.Context, pollTimeout time.Duration) (coords.Size, error) {
		waitForPipeByte(t, readyReader, readyWriter)
		cancel()
		return coords.Size{}, pollCtx.Err()
	})
	if err == nil || !strings.Contains(err.Error(), "failed to find window") {
		t.Fatalf("launchWindowedApp(canceledCtx, ..., 60s, %v) error = %v, want substring 'failed to find window'", X11CopyConfig.cmdArgs, err)
	}
	if attempts != 1 {
		t.Errorf("launchWindowedApp(canceledCtx, ..., 60s, %v) attempts = %d, want 1", X11CopyConfig.cmdArgs, attempts)
	}
	if attempt1Cmd == nil || attempt1Cmd.ProcessState == nil {
		t.Errorf("launchWindowedApp(canceledCtx, ..., 60s, %v) did not reap attempt 1 process", X11CopyConfig.cmdArgs)
	}
	if got := stderrBuf.String(); got != "attempt1-stderr" {
		t.Errorf("launchWindowedApp(canceledCtx, ..., 60s, %v) stderr = %q, want %q", X11CopyConfig.cmdArgs, got, "attempt1-stderr")
	}
}

func TestLaunchWindowedAppExhaustedRetriesAndStartFailure(t *testing.T) {
	var readyR, readyW [2]*os.File
	for i := range 2 {
		readyR[i], readyW[i] = newTestPipe(t)
	}

	attempts := 0
	guest := &fakeGuest{
		cmdFunc: func(ctx context.Context, vshArgs ...string) *testexec.Cmd {
			idx := attempts
			attempts++
			cmd := testexec.CommandContext(ctx, "sh", "-c", "printf 'err-%s' \"$1\" >&2; printf 'r' >&3; exit 0", "--", strconv.Itoa(attempts))
			cmd.ExtraFiles = []*os.File{readyW[idx]}
			return cmd
		},
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	pollIdx := 0
	_, _, _, err := launchWindowedApp(t.Context(), guest, 60*time.Second, copyAppletTitle, X11CopyConfig.cmdArgs, &stdoutBuf, &stderrBuf, func(ctx context.Context, pollTimeout time.Duration) (coords.Size, error) {
		waitForPipeByte(t, readyR[pollIdx], readyW[pollIdx])
		pollIdx++
		return coords.Size{}, errors.New("still unmapped")
	})
	if err == nil || !strings.Contains(err.Error(), "failed to find window") {
		t.Fatalf("launchWindowedApp(..., 60s, %v) error = %v, want substring 'failed to find window'", X11CopyConfig.cmdArgs, err)
	}
	if attempts != 2 {
		t.Errorf("launchWindowedApp(..., 60s, %v) attempts = %d, want 2", X11CopyConfig.cmdArgs, attempts)
	}
	if got := stderrBuf.String(); got != "err-2" {
		t.Errorf("launchWindowedApp(..., 60s, %v) stderr = %q, want %q", X11CopyConfig.cmdArgs, got, "err-2")
	}

	// Start failure should return immediately without polling or retrying.
	badCmdline := append([]string(nil), X11CopyConfig.cmdArgs...)
	pollCalls := 0
	guest.cmdFunc = func(ctx context.Context, vshArgs ...string) *testexec.Cmd {
		return testexec.CommandContext(ctx, "/nonexistent-binary-for-unit-test")
	}
	_, _, _, err = launchWindowedApp(t.Context(), guest, 60*time.Second, copyAppletTitle, badCmdline, &stdoutBuf, &stderrBuf, func(ctx context.Context, pollTimeout time.Duration) (coords.Size, error) {
		pollCalls++
		return coords.Size{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "failed to start command") {
		t.Fatalf("launchWindowedApp(..., 60s, %v) start error = %v, want substring 'failed to start command'", badCmdline, err)
	}
	if pollCalls != 0 {
		t.Errorf("launchWindowedApp(..., 60s, %v) pollCalls = %d, want 0 on Start() failure", badCmdline, pollCalls)
	}
}
