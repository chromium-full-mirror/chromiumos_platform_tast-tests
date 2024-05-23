// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CaptureWavFromPCM capture wavFile from the pcm device.
func CaptureWavFromPCM(ctx context.Context, wavFile, pcmDevice string, duration int) error {
	testing.ContextLogf(ctx, "Capturing %s directly from %s", filepath.Base(wavFile), pcmDevice)
	if err := testexec.CommandContext(ctx, "arecord", "-f", "dat", "-d", strconv.Itoa(duration), "-D"+pcmDevice, wavFile).Run(
		testexec.DumpLogOnError,
	); err != nil {
		return errors.Wrap(err, "cannot run arecord")
	}
	return nil
}

// CaptureWavFromDefault records wavFile from the default device.
func CaptureWavFromDefault(ctx context.Context, wavFile TestRawData) error {
	args := []string{
		"-b", strconv.Itoa(wavFile.BitsPerSample),
		"-e", "signed-integer",
		"-r", strconv.Itoa(wavFile.Rate),
		"-c", strconv.Itoa(wavFile.Channels),
		"-d",
		"--clobber",
		wavFile.Path,
		"trim", "0", strconv.Itoa(wavFile.Duration)}
	testing.ContextLog(ctx, "Recording with: rec ", strings.Join(args, " "))
	if err := testexec.CommandContext(
		ctx, "rec", args...,
	).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "cannot run rec")
	}
	return nil
}
