// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const aloopModuleName = "snd-aloop"

// AloopOption is a function that helps to construct snd-aloop arguments.
type AloopOption func([]string) []string

// WithMultiplePairs returns an option to enable snd-aloop with multiple device pairs.
// It appends enable=1,1 (number of pairs) to the argument list.
func WithMultiplePairs(pairs int) AloopOption {
	ones := make([]string, pairs)
	for i := 0; i < pairs; i++ {
		ones[i] = "1"
	}
	return func(args []string) []string {
		return append(args, fmt.Sprintf("enable=%s\n", strings.Join(ones, ",")))
	}
}

// LoadAloop loads snd-aloop module on kernel. A deferred call to the returned
// unloadAloop function to unload snd-aloop should be scheduled by the caller if
// err is non-nil.
func LoadAloop(ctx context.Context, options ...AloopOption) (func(context.Context), error) {
	args := []string{aloopModuleName}
	for _, opt := range options {
		args = opt(args)
	}

	if err := testexec.CommandContext(ctx, "modprobe", args...).Run(testexec.DumpLogOnError); err != nil {
		return nil, err
	}

	// For compatibility, return a cleanup function which does not expose the errors.
	return func(ctx context.Context) {
		if err := UnloadAloop(ctx); err != nil {
			testing.ContextLog(ctx, "unloadAloop() failed: ", err)
		}
	}, nil
}

// UnloadAloop unloads the snd-aloop kernel module.
func UnloadAloop(ctx context.Context) error {
	// Process cras should be stopped first, otherwise snd-aloop would not be unloaded successfully.
	if err := upstart.StopJob(ctx, "cras"); err != nil {
		return errors.Wrap(err, "failed to stop cras")
	}
	var modprobeError error
	if err := testexec.CommandContext(ctx, "modprobe", "-r", aloopModuleName).Run(testexec.DumpLogOnError); err != nil {
		modprobeError = errors.Wrapf(err, "failed to unload %s", aloopModuleName)
		testing.ContextLog(ctx, "unloadAloop(): ", modprobeError)
	}
	if err := upstart.EnsureJobRunning(ctx, "cras"); err != nil {
		return errors.Wrap(err, "failed to start cras")
	}
	return modprobeError
}
