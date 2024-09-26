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
	StyleTransferAllowed bool
	VoiceIsolation       bool

	ChromeOpts []chrome.Option
}

func installDlcs(ctx context.Context, dlcIDs []string) error {
	for _, dlcID := range dlcIDs {
		if err := dlc.Install(ctx, dlcID, ""); err != nil {
			return errors.Wrapf(err, "cannot install %s", dlcID)
		}
	}
	return nil
}

// WithNoiseCancellation setups noise cancellation
func WithNoiseCancellation(
	ctx context.Context, config NoiseCancellationConfig,
	outDir string, hasError func() bool,
	input, output string,
	f func(ctx context.Context, cras *Cras),
) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	// Start chrome.
	chromeOpts := config.ChromeOpts
	if config.StyleTransferAllowed {
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("CrOSLateBootAudioStyleTransfer"))
	} else {
		chromeOpts = append(chromeOpts, chrome.DisableFeatures("CrOSLateBootAudioStyleTransfer"))
	}

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(cleanupCtx)

	// Install DLC.
	if config.VoiceIsolation {
		if err := installDlcs(ctx, []string{"nc-ap-dlc"}); err != nil {
			return errors.Wrap(err, "failed at installing nc-ap-dlc")
		}
	}

	if err := SelectDevicesViaQuickSettings(ctx, cr, outDir, hasError, input, output); err != nil {
		return errors.Wrap(err, "failed to select loopback device from the UI")
	}

	// Start Cras.
	cras, err := NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to CRAS")
	}
	if err := cras.WaitUntilFeatureFlagHasValue(ctx, "CrOSLateBootAudioStyleTransfer", config.StyleTransferAllowed); err != nil {
		return errors.Wrap(err, "feature flag not propagated to CRAS")
	}
	if err := cras.SetVoiceIsolationUIEnabled(ctx, config.VoiceIsolation); err != nil {
		return errors.Wrap(err, "failed to SetVoiceIsolationUIEnabled")
	}

	f(ctx, cras)

	return nil
}
