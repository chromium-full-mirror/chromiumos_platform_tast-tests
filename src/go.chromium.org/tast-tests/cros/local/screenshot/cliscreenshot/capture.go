// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cliscreenshot supports taking screenshot with the CLI command.
package cliscreenshot

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// Capture takes a screenshot and saves it as a PNG image to the specified file
// path. It will use the CLI screenshot command to perform the screen capture.
func Capture(ctx context.Context, path string) error {
	cmd := testexec.CommandContext(ctx, "screenshot", path)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Errorf("failed running %q", strings.Join(cmd.Args, " "))
	}
	return nil
}

// CaptureWithStderr differs from Capture in that it returns the stderr when
// capturing a screenshot fails. This is useful for verification on whether turning display
// on/off is successful by matching with the message, "CRTC not found. Is the screen on?".
func CaptureWithStderr(ctx context.Context, path string) error {
	_, stderr, err := testexec.CommandContext(ctx, "screenshot", path).SeparatedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed running %q", stderr)
	}
	return nil
}
