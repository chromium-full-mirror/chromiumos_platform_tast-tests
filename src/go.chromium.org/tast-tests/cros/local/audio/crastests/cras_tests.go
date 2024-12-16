// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crastests

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

type cmdMode int

const (
	captureMode cmdMode = iota
	playbackMode
)

// crasTestsCommand creates a cras_tests command.
func crasTestsCommand(ctx context.Context, mode cmdMode, file string, duration, channels, rate, bufferSize int) *testexec.Cmd {
	runStr := "playback"
	if mode == captureMode {
		runStr = "capture"
	}

	cmdArgs := []string{
		runStr, file,
		"--duration", strconv.Itoa(duration),
		"--channels", strconv.Itoa(channels),
		"--rate", strconv.Itoa(rate),
	}

	if bufferSize > 0 {
		cmdArgs = append(cmdArgs, []string{"--buffer_size", strconv.Itoa(bufferSize)}...)
	}

	cmd := testexec.CommandContext(ctx, "cras_tests", cmdArgs...)

	return cmd
}

// PlaybackFileCommand creates a cras_tests playback-from-file command.
func PlaybackFileCommand(ctx context.Context, file string, duration, channels, rate int) *testexec.Cmd {
	return crasTestsCommand(ctx, playbackMode, file, duration, channels, rate, 0)
}

// PlaybackCommand creates a cras_tests playback command.
func PlaybackCommand(ctx context.Context, duration, bufferSize int) *testexec.Cmd {
	return crasTestsCommand(ctx, playbackMode, "/dev/zero", duration, 2, 48000, bufferSize)
}

// CaptureFileCommand creates a cras_tests capture-to-file command.
func CaptureFileCommand(ctx context.Context, file string, duration, channels, rate int) *testexec.Cmd {
	return crasTestsCommand(ctx, captureMode, file, duration, channels, rate, 0)
}

// CaptureCommand creates a cras_tests capture command.
func CaptureCommand(ctx context.Context, duration, bufferSize int) *testexec.Cmd {
	return crasTestsCommand(ctx, captureMode, "/dev/null", duration, 2, 48000, bufferSize)
}
