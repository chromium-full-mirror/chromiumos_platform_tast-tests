// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// GBB represents basic GBB fields.
type GBB struct {
	HWID   string
	Flags  int
	Digest string
}

// GetGBBOptions holds configuration options for GetGBB().
type GetGBBOptions struct {
	rootKey     string
	recoveryKey string
}

// WithRootKey configures Root Key output path.
func (o *GetGBBOptions) WithRootKey(rootKey string) *GetGBBOptions {
	o.rootKey = rootKey
	return o
}

// WithRecoveryKey configures Recovery Key output path.
func (o *GetGBBOptions) WithRecoveryKey(recoveryKey string) *GetGBBOptions {
	o.recoveryKey = recoveryKey
	return o
}

// GetGBB is general wrapper for `futility gbb --get`.
//
// On success returns GBB and program output. On failure returns error.
func (i *Instance) GetGBB(ctx context.Context, opts *GetGBBOptions) (*GBB, []byte, error) {
	cmdArgs := append(i.futilityCmdArgs(), "gbb", "--get", "--hwid", "--flags", "--digest")

	if opts.rootKey != "" {
		cmdArgs = append(cmdArgs, "--rootkey", opts.rootKey)
	}
	if opts.recoveryKey != "" {
		cmdArgs = append(cmdArgs, "--recoverykey", opts.recoveryKey)
	}

	cmdArgs = i.appendFlashArgs(cmdArgs)
	cmdArgs = append(cmdArgs, "--flash")

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return nil, fullOut, errors.Wrapf(err, "failed to get GBB with arguments %v", cmdArgs)
	}

	strout := string(stdout)
	gbb := &GBB{}

	flagsRe := regexp.MustCompile(`flags:\s*(0[xX][0-9a-fA-F]+)`)
	result := flagsRe.FindStringSubmatch(strout)
	if result == nil || len(result) < 2 {
		return nil, fullOut, errors.Errorf("flags not present in GBB output with arguments %v", cmdArgs)
	}
	flags, err := strconv.ParseInt(result[1], 0, 0)
	if err != nil {
		return nil, fullOut, errors.Errorf("GBB flags in incorrect format with arguments %v", cmdArgs)
	}
	gbb.Flags = int(flags)

	hwidRe := regexp.MustCompile(`hardware_id:([^\r\n]+)`)
	result = hwidRe.FindStringSubmatch(strout)
	if result == nil || len(result) < 2 {
		return nil, fullOut, errors.Errorf("HWID not present in GBB output with arguments %v", cmdArgs)
	}
	gbb.HWID = strings.TrimSpace(result[1])

	digestRe := regexp.MustCompile(`digest:\s*([0-9a-f-A-F]+)`)
	result = digestRe.FindStringSubmatch(strout)
	if result == nil || len(result) < 2 {
		return nil, fullOut, errors.Errorf("digest not present in GBB output with arguments %v", cmdArgs)
	}
	gbb.Digest = result[1]

	return gbb, fullOut, nil
}

// GetGBBFlags extracts GBB flags from firmware.
//
// Returns GBB flags on success. Returns zero and error on failure.
func (i *Instance) GetGBBFlags(ctx context.Context) (int, error) {
	gbb, output, err := i.GetGBB(ctx, &GetGBBOptions{})

	if err != nil {
		return 0, errors.Wrapf(err, "failed to get GBB flags, output: %q", string(output))
	}

	return gbb.Flags, nil
}

// SetGBBOptions holds configuration options for SetGBB().
type SetGBBOptions struct {
	hwid        string
	flags       int
	hasFlags    bool
	rootKey     string
	recoveryKey string
}

// WithHWID configures Hardware ID (HWID) to be set.
func (o *SetGBBOptions) WithHWID(hwid string) *SetGBBOptions {
	o.hwid = hwid
	return o
}

// WithFlags configures GBB flags to be set.
func (o *SetGBBOptions) WithFlags(flags int) *SetGBBOptions {
	o.flags = flags
	o.hasFlags = true
	return o
}

// WithRootKey configures Root Key path to be set.
func (o *SetGBBOptions) WithRootKey(rootKey string) *SetGBBOptions {
	o.rootKey = rootKey
	return o
}

// WithRecoveryKey configures Recovery Key path to be set.
func (o *SetGBBOptions) WithRecoveryKey(recoveryKey string) *SetGBBOptions {
	o.recoveryKey = recoveryKey
	return o
}

// SetGBB is general wrapper for `futility gbb --set`.
//
// Returns program output on success.
func (i *Instance) SetGBB(ctx context.Context, opts *SetGBBOptions) ([]byte, error) {
	cmdArgs := append(i.futilityCmdArgs(), "gbb", "--set")

	if opts.hasFlags {
		cmdArgs = append(cmdArgs, "--flags", fmt.Sprintf("%#x", opts.flags))
	}
	if opts.hwid != "" {
		cmdArgs = append(cmdArgs, "--hwid", opts.hwid)
	}
	if opts.rootKey != "" {
		cmdArgs = append(cmdArgs, "--rootkey", opts.rootKey)
	}
	if opts.recoveryKey != "" {
		cmdArgs = append(cmdArgs, "--recoverykey", opts.recoveryKey)
	}

	cmdArgs = i.appendFlashArgs(cmdArgs)
	cmdArgs = append(cmdArgs, "--flash")

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "failed to set GBB with arguments %v", cmdArgs)
	}

	return fullOut, nil
}

// SetGBBFlags sets GBB flags in firmware.
func (i *Instance) SetGBBFlags(ctx context.Context, flags int) error {
	opts := (&SetGBBOptions{}).WithFlags(flags)
	output, err := i.SetGBB(ctx, opts)

	if err != nil {
		return errors.Wrapf(err, "failed to set GBB flags, output: %q", string(output))
	}

	return nil
}

// CreateGBB is general wrapper for `futility gbb --create`.
//
// outFile is required path to the GBB blob output file.
// hwidSize, rootKeySize, bmpfvSize, recoveryKeySize are sizes of HWID,
// Root Key and Recovery Key sizes in new GBB.
//
// Returns program output on success and error on failure.
func (i *Instance) CreateGBB(ctx context.Context, outFile string, hwidSize, rootKeySize, recoveryKeySize int) ([]byte, error) {
	if outFile == "" {
		return nil, errors.New("cannot create GBB blob, output file not passed")
	}

	cmdArgs := append(i.futilityCmdArgs(), "gbb", "--create",
		fmt.Sprintf("%#x,%#x,0,%#x", hwidSize, rootKeySize, recoveryKeySize),
		outFile)

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "failed to create GBB with arguments %v", cmdArgs)
	}

	return fullOut, nil
}
