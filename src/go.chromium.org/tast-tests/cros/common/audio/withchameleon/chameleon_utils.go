// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package withchameleon contains useful abstraction on RPC interfaces for
// chameleond devices.
package withchameleon

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PlaybackInitSleepDuration is the time to buffer after playback started
const PlaybackInitSleepDuration = 500 * time.Millisecond

// RecordStopDuration is the time to stop recording before playback ends
const RecordStopDuration = 500 * time.Millisecond

// PlayFileByPortType plays an audio stream (specified by a token) to a
// designated port in Chameleon.
// The token can be obtained by CopyFileToChameleon(), and
// a) decoupled how the audio data is internally stored in chameleon with how
// it's going to be played
// b) eliminated the usage of SSH connection, which will be compatible with v3.
// See more at b/262479811 and b/234744284
func PlayFileByPortType(ctx context.Context, chameleond chameleon.Chameleond, token string, portType chameleon.PortType, audioFormat *chameleon.AudioDataFormat) error {
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	portID, err := chameleond.FetchSupportedPortIDByType(ctx, portType, 0)
	if err != nil {
		return errors.Wrapf(err, "failed to get port id of portType: %s", portType.String())
	}
	_, err = chameleond.ProbeOutputs(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to ProbeOutputs")
	}
	hasAudioSupport, err := chameleond.HasAudioSupport(ctx, portID)
	if err != nil || !hasAudioSupport {
		return errors.Wrap(err, "has no audio support")
	}
	_, err = chameleond.GetConnectorType(ctx, portID)
	if err != nil {
		return errors.Wrap(err, "unable to get connector type")
	}
	if err = chameleond.Plug(ctx, portID); err != nil {
		return errors.Wrap(err, "unable to plug the port")
	}
	if err = chameleond.SetUSBDriverPlaybackConfigs(ctx, audioFormat); err != nil {
		return errors.Wrap(err, "failed to set the USB driver playback config")
	}

	if err = chameleond.StartPlayingAudioWithToken(ctx, portID, token, audioFormat); err != nil {
		return errors.Wrap(err, "failed when calling StartPlayingAudioWithToken")
	}

	// GoBigSleepLint wait chameleon to start playing audio
	testing.Sleep(ctx, PlaybackInitSleepDuration)

	return nil
}

// CalculateRecordDuration used playbackDuration to calculate the require time
// to sleep while waiting for recording.
// If recordDuration is too small, it default the duration to 1 second.
func CalculateRecordDuration(playbackDuration time.Duration) time.Duration {
	recordDuration := playbackDuration - PlaybackInitSleepDuration - RecordStopDuration
	if recordDuration < time.Second {
		recordDuration = time.Second
	}
	return recordDuration
}
