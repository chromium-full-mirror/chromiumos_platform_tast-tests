// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mlbenchmark contains helper functions to easily create
// new ML based benchmarks.
package mlbenchmark

import (
	"context"
	"fmt"
	"math/big"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// DataDirectory is the location to unpack any associated data files into.
	DataDirectory = "/usr/local/mlbenchmark/data"
)

// UnpackData will untar the file specified by `dataPath` into `DataDirectory`.
func UnpackData(ctx context.Context, dataPath string) error {
	testing.ContextLogf(ctx, "unpacking %s into %s", dataPath, DataDirectory)
	tarCmd := testexec.CommandContext(ctx, "tar", "-xvf", dataPath, "-C", DataDirectory)
	if err := tarCmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to untar test artifacts")
	}

	return nil
}

// DataPath returns the absolute path for a data file that was extracted with `UnpackData`.
func DataPath(f string) string {
	return filepath.Join(DataDirectory, f)
}

// BuildCommand will construct a `Cmd` object using `execName` as the binary and `args` as the
// set of arguments to pass to the executable.
func BuildCommand(ctx context.Context, execName string, args map[string]string) *testexec.Cmd {
	var argsAsStr = []string{}
	for key, value := range args {
		argsAsStr = append(argsAsStr, fmt.Sprintf("%s=%s", key, value))
	}
	return testexec.CommandContext(ctx, execName, argsAsStr...)
}

// ParseNumeric will parse a float64 from an input string. Supports scientific notation.
func ParseNumeric(input string) (float64, error) {
	flt, _, err := big.ParseFloat(input, 10, 0, big.ToNearestEven)
	if err != nil {
		return 0, errors.Wrapf(err, "couldn't parse input: %s", input)
	}
	f, _ := flt.Float64()
	return f, nil
}
