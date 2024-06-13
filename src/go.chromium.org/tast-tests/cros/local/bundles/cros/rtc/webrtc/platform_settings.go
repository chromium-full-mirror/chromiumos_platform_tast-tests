// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/effects"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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

// enableCameraEffects turns on the platform video conferencing effects (platform blurring, relighting, and face retouching).
// The effects are reset when the returned function is invoked.
func enableCameraEffects(ctx context.Context, blur, relight, retouch bool) (func(), error) {
	blurLevel := effects.KBlurDisabled
	if blur {
		blurLevel = effects.KBlurMedium
	}
	resetEffects, err := effects.ApplyPlatformEffects(ctx, relight, retouch, blurLevel, effects.KAuto)
	if err != nil {
		return nil, errors.Wrap(err, "failed to enable platform blurring")
	}
	return func() {
		if err := resetEffects(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to reset platform effects: ", err)
		}
	}, nil
}
