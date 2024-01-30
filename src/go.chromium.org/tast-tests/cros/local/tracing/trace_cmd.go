// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tracing

import (
	"bytes"
	"context"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

// RunTraceCmd starts "trace-cmd record" process. It returns cleanupFunc, which
// kills the trace-cmd process, and collectFunc, which stops the trace-cmd
// process to collect recorded trace data.
//
// A caller is expected to make sure cleanupFunc even for the error cases.
// Otherwise, trace-cmd may keep running even after the test exits. So,
// cleanupFunc is supposed to be called with defer like the following example:
//
//	  cleanupFunc, collectFunc, err := tracing.RunTraceCmd(ctx, "/path/to/trace.dat", []string{"syscalls"})
//	  if err != nil {
//		   s.Fatal("Failed to start trace-cmd", err)
//	  }
//	  defer cleanupFunc() // Make sure calling cleanupFunc() even on failures
//	  <logic to be traced>
//	  if err != collectFunc(ctx) {
//	    s.Fatal("Failed to collect trace.dat", err)
//	  }
func RunTraceCmd(ctx context.Context, outPath string, events []string) (cleanupFunc func(), collectFunc func(context.Context) error, err error) {
	args := []string{"record", "-b", "15000", "-o", outPath}
	for _, e := range events {
		args = append(args, "-e", e)
	}
	traceCmd := testexec.CommandContext(ctx, "trace-cmd", args...)
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	traceCmd.Stdout = &stdout
	traceCmd.Stderr = &stderr
	if err = traceCmd.Start(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to start trace-cmd on the host")
	}

	cleanupFunc = func() {
		traceCmd.Kill()
	}
	collectFunc = func(ctx context.Context) error {
		if err := traceCmd.Signal(unix.SIGINT); err != nil {
			return errors.Wrap(err, "failed to kill trace-cmd")
		}
		if err := traceCmd.Wait(); err != nil {
			return errors.Wrapf(err, "failed to wait or trace-cmd exited with error. stdout: %q, stderr: %q", stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String(), "events lost") {
			return errors.Errorf("trace-cmd lost events. Try increasing the buffer size of trace-cmd with -b option. stdout: %q, stderr: %q", stdout.String(), stderr.String())
		}
		if _, err := os.Stat(outPath); err != nil {
			return errors.Wrapf(err, "trace-cmd didn't produce data at %s", outPath)
		}
		testing.ContextLogf(ctx, "trace-cmd record succeeded with %s generated", outPath)

		return nil
	}
	return
}
