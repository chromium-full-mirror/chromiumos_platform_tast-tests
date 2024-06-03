// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"

	"go.chromium.org/tast/core/errors"
)

// showOptions holds configuration options for Show().
type showOptions struct {
	path string
}

// NewShowOptions returns new showOptions with input path.
func NewShowOptions(path string) *showOptions {
	return &showOptions{
		path: path,
	}
}

// Show calls `futility show`
//
// Returns program output, and error on failure.
func (i *Instance) Show(ctx context.Context, opts *showOptions) ([]byte, error) {
	if opts.path == "" {
		return nil, errors.New("no path to show")
	}

	cmdArgs := append(i.futilityCmdArgs(), "show", opts.path, "--parseable")

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "failed to show the path with arguments %v", cmdArgs)
	}

	return fullOut, nil
}
