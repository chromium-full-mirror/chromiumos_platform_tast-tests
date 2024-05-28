// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// NoiseCancellationConfig struct
type NoiseCancellationConfig struct {
	NoiseCancellationEnabled bool
	StyleTransferEnabled     bool

	ChromeOpts []chrome.Option
}

// WithNoiseCancellation setups noise cancellation
func WithNoiseCancellation(ctx context.Context, config NoiseCancellationConfig, outDir string, hasError func() bool, f func(ctx context.Context)) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	// Start chrome.
	chromeOpts := config.ChromeOpts
	if config.StyleTransferEnabled {
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("CrOSLateBootAudioStyleTransfer"))
	}

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(cleanupCtx)

	// Install DLC.
	if config.NoiseCancellationEnabled {
		if err := dlc.Install(ctx, "nc-ap-dlc", ""); err != nil {
			return errors.Wrap(err, "cannot install nc-ap-dlc")
		}
	}
	if config.StyleTransferEnabled {
		if err := dlc.Install(ctx, "nuance-dlc", ""); err != nil {
			return errors.Wrap(err, "cannot install nuance-dlc")
		}
	}

	if err := SetupLoopback(ctx, cr, outDir, hasError); err != nil {
		return errors.Wrap(err, "failed to select loopback device from the UI")
	}

	// Start Cras.
	cras, err := NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to CRAS")
	}
	if config.StyleTransferEnabled {
		if err := cras.WaitUntilFeatureFlagHasValue(ctx, "CrOSLateBootAudioStyleTransfer", true); err != nil {
			return errors.Wrap(err, "feature flag not propagated to CRAS")
		}
	}
	if err := cras.SetNoiseCancellationEnabled(ctx, config.NoiseCancellationEnabled); err != nil {
		return errors.Wrap(err, "failed to SetNoiseCancellationEnabled")
	}
	if err := cras.SetStyleTransferEnabled(ctx, config.StyleTransferEnabled); err != nil {
		return errors.Wrap(err, "failed to SetStyleTransferEnabled")
	}

	f(ctx)

	return nil
}
