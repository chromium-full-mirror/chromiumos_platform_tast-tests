// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"

	"go.chromium.org/tast/core/errors"
)

// readAPOptions holds configuration options for ReadAP().
type readAPOptions struct {
	outFile string
	region  string
	debug   bool
	verbose bool
}

// NewReadAPOptions returns new readAPOptions with output file path set.
func NewReadAPOptions(outFile string) *readAPOptions {
	return &readAPOptions{
		outFile: outFile,
	}
}

// WithRegion configures region to be read by ReadAP().
// Empty means whole image.
func (o *readAPOptions) WithRegion(region string) *readAPOptions {
	o.region = region
	return o
}

// WithDebug configures debug output.
func (o *readAPOptions) WithDebug(debug bool) *readAPOptions {
	o.debug = debug
	return o
}

// WithVerbose configures verbose output.
func (o *readAPOptions) WithVerbose(verbose bool) *readAPOptions {
	o.verbose = verbose
	return o
}

// ReadAP reads AP firmware to file.
//
// Returns program output, and error on failure.
func (i *Instance) ReadAP(ctx context.Context, opts *readAPOptions) ([]byte, error) {
	if opts.outFile == "" {
		return nil, errors.New("cannot read AP firmware: output file not available")
	}

	cmdArgs := append(i.futilityCmdArgs(), "read")
	if opts.debug {
		cmdArgs = append(cmdArgs, "--debug")
	}
	if opts.verbose {
		cmdArgs = append(cmdArgs, "--verbose")
	}
	if opts.region != "" {
		cmdArgs = append(cmdArgs, "--region", opts.region)
	}

	cmdArgs = i.appendFlashArgs(cmdArgs)

	cmdArgs = append(cmdArgs, opts.outFile)

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "failed to read AP firmware with arguments %v", cmdArgs)
	}

	return fullOut, nil
}
