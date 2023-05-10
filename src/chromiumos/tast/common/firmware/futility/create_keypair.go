// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"fmt"

	"go.chromium.org/tast/core/errors"
)

// HashAlgorithm describes hashing algorithm to use for keypair generation.
type HashAlgorithm int

// HashAlgorithm values.
const (
	HashAlgorithmNone   HashAlgorithm = 0
	HashAlgorithmSHA1   HashAlgorithm = 1
	HashAlgorithmSHA256 HashAlgorithm = 2
	HashAlgorithmSHA512 HashAlgorithm = 3
	HashAlgorithmSHA224 HashAlgorithm = 4
	HashAlgorithmSHA384 HashAlgorithm = 5
)

// createKeyPairOptions holds options for CreateKeyPair().
type createKeyPairOptions struct {
	inFile        string
	basename      string
	version       int
	hasVersion    bool
	hashAlgorithm HashAlgorithm
	id            string // vb2_id hex string
	description   string
}

// NewCreateKeyPairOptions creates new options instance for CreateKeyPair().
func NewCreateKeyPairOptions(inFile string) *createKeyPairOptions {
	return &createKeyPairOptions{
		inFile:        inFile,
		hashAlgorithm: -1, // -1 indicates no algorithm, as it is not a valid value.
	}
}

// WithBaseName configures output filenames prefix.
func (o *createKeyPairOptions) WithBaseName(basename string) *createKeyPairOptions {
	o.basename = basename
	return o
}

// WithVersion configures key version.
func (o *createKeyPairOptions) WithVersion(version int) *createKeyPairOptions {
	o.version = version
	o.hasVersion = true
	return o
}

// WithHashAlgorithm configures hashing algorithm for key generation.
func (o *createKeyPairOptions) WithHashAlgorithm(algorithm HashAlgorithm) *createKeyPairOptions {
	o.hashAlgorithm = algorithm
	return o
}

// WithID configures keypair identifier.
func (o *createKeyPairOptions) WithID(id string) *createKeyPairOptions {
	o.id = id
	return o
}

// WithDescription configures human-readable description.
func (o *createKeyPairOptions) WithDescription(description string) *createKeyPairOptions {
	o.description = description
	return o
}

// CreateKeyPair creates new key pair form RSA key (.pem file).
//
// This function calls `futility create` configured with provided options.
// Caller has to cleanup all resulting files themselves.
//
// Returns program output, and error on failure.
func (i *Instance) CreateKeyPair(ctx context.Context, opt *createKeyPairOptions) ([]byte, error) {
	if opt.inFile == "" {
		return nil, errors.New("input file cannot be empty")
	}

	cmdArgs := append(i.futilityCmdArgs(), "create")

	if opt.hasVersion {
		cmdArgs = append(cmdArgs, "--version", fmt.Sprintf("%d", opt.version))
	}
	if int(opt.hashAlgorithm) != -1 {
		cmdArgs = append(cmdArgs, "--hash_alg", fmt.Sprintf("%d", int(opt.hashAlgorithm)))
	}
	if opt.id != "" {
		cmdArgs = append(cmdArgs, "--id", opt.id)
	}
	if opt.description != "" {
		cmdArgs = append(cmdArgs, "--desc", opt.description)
	}

	cmdArgs = append(cmdArgs, opt.inFile)
	if opt.basename != "" {
		cmdArgs = append(cmdArgs, opt.basename)
	}

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "failed to create key pair with arguments: %v", cmdArgs)
	}

	return fullOut, nil
}
