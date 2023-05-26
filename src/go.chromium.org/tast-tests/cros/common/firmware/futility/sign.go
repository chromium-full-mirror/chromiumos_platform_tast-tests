// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"fmt"

	"go.chromium.org/tast/core/errors"
)

// signBIOSOptions holds configuration options for SignBIOS().
type signBIOSOptions struct {
	inFile      string
	outFile     string
	keyset      string
	signPrivate string
	keyBlock    string
	kernelKey   string
	flags       int
	hasFlags    bool
	version     int
	hasVersion  bool
	localOEMDir string
	localOEMID  string
}

// NewSignBIOSOptions returns new signBIOSOptions with input (BIOS) file path.
func NewSignBIOSOptions(inFile string) *signBIOSOptions {
	return &signBIOSOptions{
		inFile: inFile,
	}
}

// WithOutputFile configures output (bios) file path.
func (o *signBIOSOptions) WithOutputFile(outFile string) *signBIOSOptions {
	o.outFile = outFile
	return o
}

// WithKeysetPath configures keyset directory path.
func (o *signBIOSOptions) WithKeysetPath(keyset string) *signBIOSOptions {
	o.keyset = keyset
	return o
}

// WithSignPrivatePath configures private data key path.
func (o *signBIOSOptions) WithSignPrivatePath(signPrivate string) *signBIOSOptions {
	o.signPrivate = signPrivate
	return o
}

// WithKeyBlockPath configures keyblock path.
func (o *signBIOSOptions) WithKeyBlockPath(keyBlock string) *signBIOSOptions {
	o.keyBlock = keyBlock
	return o
}

// WithKernelKeyPath configures public kernel subkey.
func (o *signBIOSOptions) WithKernelKeyPath(kernelKey string) *signBIOSOptions {
	o.kernelKey = kernelKey
	return o
}

// WithFlags configures preamble flags to be set.
func (o *signBIOSOptions) WithFlags(flags int) *signBIOSOptions {
	o.flags = flags
	o.hasFlags = true
	return o
}

// WithVersion configures firmware version in preamble to be set.
func (o *signBIOSOptions) WithVersion(version int) *signBIOSOptions {
	o.version = version
	o.hasVersion = true
	return o
}

// WithLocalOEMDir configures output directory path for Local OEM vblock.
func (o *signBIOSOptions) WithLocalOEMDir(localOEMDir string) *signBIOSOptions {
	o.localOEMDir = localOEMDir
	return o
}

// WithLocalOEMID configures Local ID vblock suffix.
func (o *signBIOSOptions) WithLocalOEMID(localOEMID string) *signBIOSOptions {
	o.localOEMID = localOEMID
	return o
}

// SignBIOS calls `futility sign --type bios`
//
// Returns program output, and error on failure.
func (i *Instance) SignBIOS(ctx context.Context, opts *signBIOSOptions) ([]byte, error) {
	if opts.inFile == "" {
		return nil, errors.New("no file to sign")
	}

	cmdArgs := append(i.futilityCmdArgs(), "sign", "--type", "bios")
	if opts.keyset != "" {
		cmdArgs = append(cmdArgs, "--keyset", opts.keyset)
	}
	if opts.signPrivate != "" {
		cmdArgs = append(cmdArgs, "--signprivate", opts.signPrivate)
	}
	if opts.keyBlock != "" {
		cmdArgs = append(cmdArgs, "--keyblock", opts.keyBlock)
	}
	if opts.kernelKey != "" {
		cmdArgs = append(cmdArgs, "--kernelkey", opts.kernelKey)
	}
	if opts.hasVersion {
		cmdArgs = append(cmdArgs, "--version", fmt.Sprintf("%d", opts.version))
	}
	if opts.hasFlags {
		cmdArgs = append(cmdArgs, "--flags", fmt.Sprintf("%#x", opts.flags))
	}
	if opts.localOEMDir != "" {
		cmdArgs = append(cmdArgs, "--loemdir", opts.localOEMDir)
	}
	if opts.localOEMID != "" {
		cmdArgs = append(cmdArgs, "--loemid", opts.localOEMID)
	}

	cmdArgs = append(cmdArgs, opts.inFile)
	if opts.outFile != "" {
		cmdArgs = append(cmdArgs, opts.outFile)
	}

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "failed to sign BIOS image with arguments %v", cmdArgs)
	}

	return fullOut, nil
}

// signRWSigOptions holds configuration for SignRWSig().
type signRWSigOptions struct {
	inFile     string
	outFile    string
	privKey    string
	version    int
	hasVersion bool
	sigSize    int
	dataSize   int
}

// NewSignRWSigOptions returns new signRWSigOptions with input file path set.
func NewSignRWSigOptions(inFile string) *signRWSigOptions {
	return &signRWSigOptions{
		inFile: inFile,
	}
}

// WithOutputFile configures output file path.
func (o *signRWSigOptions) WithOutputFile(outFile string) *signRWSigOptions {
	o.outFile = outFile
	return o
}

// WithPrivateKeyPath configures private key path.
func (o *signRWSigOptions) WithPrivateKeyPath(privKey string) *signRWSigOptions {
	o.privKey = privKey
	return o
}

// WithVersion configures public key version.
func (o *signRWSigOptions) WithVersion(version int) *signRWSigOptions {
	o.version = version
	o.hasVersion = true
	return o
}

// WithSignatureSize configures maximum signature size.
// Offset from the end of input file, where signature blob should be located
// if file does not contain FlashMap.
func (o *signRWSigOptions) WithSignatureSize(sigSize int) *signRWSigOptions {
	o.sigSize = sigSize
	return o
}

// WithDataSize configures number of bytes in input file to sign.
func (o *signRWSigOptions) WithDataSize(dataSize int) *signRWSigOptions {
	o.dataSize = dataSize
	return o
}

// SignRWSig calls `futility sign --type rwsig`
//
// Returns program output, and error on failure.
func (i *Instance) SignRWSig(ctx context.Context, opts *signRWSigOptions) ([]byte, error) {
	if opts.inFile == "" {
		return nil, errors.New("no file to sign")
	}

	cmdArgs := append(i.futilityCmdArgs(), "sign", "--type", "rwsig")
	if opts.privKey != "" {
		cmdArgs = append(cmdArgs, "--prikey", opts.privKey)
	}
	if opts.hasVersion {
		cmdArgs = append(cmdArgs, "--version", fmt.Sprintf("%d", opts.version))
	}
	if opts.sigSize > 0 {
		cmdArgs = append(cmdArgs, "--sig_size", fmt.Sprintf("%d", opts.sigSize))
	}
	if opts.dataSize > 0 {
		cmdArgs = append(cmdArgs, "--data_size", fmt.Sprintf("%d", opts.dataSize))
	}

	cmdArgs = append(cmdArgs, opts.inFile)
	if opts.outFile != "" {
		cmdArgs = append(cmdArgs, opts.outFile)
	}

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "failed to read AP firmware with arguments %v", cmdArgs)
	}

	return fullOut, nil
}
