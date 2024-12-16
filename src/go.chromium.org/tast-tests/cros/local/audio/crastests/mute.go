// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crastests provides functions to interact cras_tests
package crastests

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

// Mute lets DUT be muted. That is, after Mute() is done, DUT doesn't sound when a video plays.
func Mute(ctx context.Context) error {
	cmd := testexec.CommandContext(ctx, "cras_tests", "control", "set_mute", "true")
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return err
	}
	return nil
}

// Unmute lets DUT be unmuted, if it is muted by Mute().
func Unmute(ctx context.Context) error {
	cmd := testexec.CommandContext(ctx, "cras_tests", "control", "set_mute", "false")
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return err
	}
	return nil
}
