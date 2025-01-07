// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// LabelAudioBeamforming returns the beamforming type.
//
//   - intelligo: Intelligo beamforming.
//   - none: Beamforming unsupported.
func LabelAudioBeamforming(ctx context.Context) (string, error) {
	stdout, err := testexec.CommandContext(ctx, "cras_server_tool", "label-audio_beamforming").Output(testexec.DumpLogOnError)
	return strings.TrimRight(string(stdout), "\n"), err
}

// CheckBeamforming returns an error if beamforming is not supported on the device.
func CheckBeamforming(ctx context.Context) error {
	bf, err := LabelAudioBeamforming(ctx)
	if err != nil {
		return errors.Wrap(err, "audio.LabelAudioBeamforming")
	}
	if bf != "intelligo" {
		return errors.Errorf("beamforming not supported: `cras_server_tool label-audio_beamforming` reports %q", bf)
	}
	return nil
}
