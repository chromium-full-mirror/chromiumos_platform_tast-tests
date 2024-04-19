// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/errors"
)

// setUpAudio configures the audio server according to noiseCancellation and styleTransfer.
func setUpAudio(ctx context.Context, noiseCancellation, styleTransfer bool) error {
	if noiseCancellation {
		if err := dlc.Install(ctx, "nc-ap-dlc", ""); err != nil {
			return errors.Wrap(err, "cannot install nc-ap-dlc")
		}
	}
	cras, err := audio.RestartCras(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot restart CRAS")
	}

	if err := audio.SelectIODevices(ctx, cras, "INTERNAL_MIC", "INTERNAL_SPEAKER"); err != nil {
		return errors.Wrap(err, "audio.SelectIODevices")
	}

	if err := cras.SetNoiseCancellationEnabled(ctx, noiseCancellation); err != nil {
		return errors.Wrap(err, "cras.SetNoiseCancellationEnabled")
	}

	if err := cras.SetStyleTransferEnabled(ctx, styleTransfer); err != nil {
		return errors.Wrap(err, "cras.SetStyleTransferEnabled")
	}
	return nil
}
