// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PlayWavToPCM plays wavFile to the pcm device.
func PlayWavToPCM(ctx context.Context, wavFile, pcmDevice string) error {
	testing.ContextLogf(ctx, "Playing %s directly to %s", filepath.Base(wavFile), pcmDevice)
	if err := testexec.CommandContext(ctx, "aplay", "-D"+pcmDevice, wavFile).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "cannot run aplay")
	}
	return nil
}

// PlayWavToDefault plays wavFile to the default device.
func PlayWavToDefault(ctx context.Context, wavFile string) error {
	testing.ContextLogf(ctx, "Playing %s through play command", filepath.Base(wavFile))
	if err := testexec.CommandContext(ctx, "play", wavFile).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "cannot run play")
	}
	return nil
}
