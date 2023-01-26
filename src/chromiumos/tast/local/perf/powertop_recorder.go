// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// PowertopRecorder runs `powertop` in background to generate reports and store
// them under an output dir.
type PowertopRecorder struct {
	outDir string
	args   []string
	cmd    *testexec.Cmd
}

// NewPowertopRecorder creates a new instance of PowertopRecorder.
func NewPowertopRecorder(ctx context.Context, interval time.Duration, outDir string) (*PowertopRecorder, error) {
	// Ensures that the output dir exists.
	if err := os.Mkdir(outDir, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create powertop output directory")
	}

	// maxTestDurationSeconds is an arbitrary long time that should be longer than
	// all tests so that `powertop` does not exit before test ends.
	const maxTestDurationSeconds = 3600

	iterations := int(maxTestDurationSeconds / interval.Seconds())

	return &PowertopRecorder{
		outDir: outDir,
		args: []string{
			"-C", "powertop.csv",
			"-i", fmt.Sprint(iterations),
			"-t", fmt.Sprint(int(interval.Seconds())),
		},
	}, nil
}

// Start starts `powertop` in background.
func (t *PowertopRecorder) Start(ctx context.Context) error {
	if t.cmd != nil {
		return errors.New("PowertopRecorder already started")
	}

	t.cmd = testexec.CommandContext(ctx, "powertop", t.args...)

	// Change work dir to `outDir` to write result files to it. Passing full path
	// in `-C` does not work with temp tast out dir.
	t.cmd.Cmd.Dir = t.outDir

	return t.cmd.Start()
}

// Stop stops the running `powertop`.
func (t *PowertopRecorder) Stop(ctx context.Context) error {
	if err := t.cmd.Signal(unix.SIGTERM); err != nil {
		return errors.Wrap(err, "failed to stop powertop")
	}
	if err := t.cmd.Wait(); err != nil {
		// SIGTERM always causes Wait() to return an error.
		testing.ContextLogf(ctx, "powertop termination wait failed (%q)", err)
	}
	t.cmd = nil
	return nil
}
