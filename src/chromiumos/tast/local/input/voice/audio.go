// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package voice provides functionality related to voice inputs
package voice

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/audio"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/chrome/uiauto/role"
)

// LoopbackDevice represents the loopback device type.
type LoopbackDevice string

// Available options of LoopbackDevice.
const (
	LoopbackPlayBack LoopbackDevice = "Loopback Playback"
	LoopbackCapture  LoopbackDevice = "Loopback Capture"
)

// AudioFromFile inputs an audio file via active input node and waits for its completion.
func AudioFromFile(ctx context.Context, audioFilePath string) error {
	audioInput := audio.TestRawData{
		Path:          audioFilePath,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Frequencies:   []int{440, 440},
		Volume:        0.05,
	}

	// Playback function by CRAS.
	playCmd := crastestclient.PlaybackFileCommand(
		ctx, audioInput.Path,
		audioInput.Duration,
		audioInput.Channels,
		audioInput.Rate)
	playCmd.Start()

	return playCmd.Wait()
}

// EnableAloop loads and enables Aloop then sets it as active input/output node.
func EnableAloop(ctx context.Context, tconn *chrome.TestConn) (func(ctx context.Context), error) {
	// Load ALSA loopback module.
	unload, err := audio.LoadAloop(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load ALSA loopback module")
	}

	// Activate the Aloop nodes.
	if err := ActivateAloopNodes(ctx, tconn, LoopbackPlayBack, LoopbackCapture); err != nil {
		// Unload ALSA loopback if any following setups failed.
		unload(ctx)
		return nil, err
	}

	return unload, nil
}

// ActivateAloopNodes activates Aloop nodes as input/output devices.
// Switching nodes via UI interactions is the recommended way, instead of using
// cras.SetActiveNode() method, as UI will always send the preference input/output
// devices to CRAS. Calling cras.SetActiveNode() changes the active devices for a
// moment, but they soon are reverted by UI. See (b/191602192) for details.
func ActivateAloopNodes(ctx context.Context, tconn *chrome.TestConn, devices ...LoopbackDevice) error {
	cleanupCtx := ctx
	ctx, shortCancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer shortCancel()
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to show Quick Settings")
	}
	defer quicksettings.Hide(cleanupCtx, tconn)

	if len(devices) == 0 {
		return errors.New("no device to be activated")
	}

	for _, device := range devices {
		if err := selectAudioOption(ctx, tconn, string(device)); err != nil {
			return errors.Wrapf(err, "failed to select ALSA %q", string(device))
		}
	}
	return nil
}

func selectAudioOption(ctx context.Context, tconn *chrome.TestConn, device string) error {
	if err := quicksettings.OpenAudioSettings(ctx, tconn); err != nil {
		return err
	}

	ui := uiauto.New(tconn)
	option := nodewith.Role(role.CheckBox).Name(device)

	if err := ui.DoDefault(option)(ctx); err != nil {
		return errors.Wrapf(err, "failed to click %v audio option", device)
	}

	return nil
}
