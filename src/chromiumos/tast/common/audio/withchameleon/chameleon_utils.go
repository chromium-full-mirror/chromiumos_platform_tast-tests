// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package withchameleon contains useful abstraction on RPC interfaces for
// chameleond devices.
package withchameleon

import (
	"context"
	"time"

	"chromiumos/tast/common/chameleon"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PlayFileByPortType plays an audio stream (specified by a token) to a
// designated port in Chameleon. It plays for playbackDuration seconds and then stop.
// The token can be obtained by CopyFileToChameleon(), and
// a) decoupled how the audio data is internally stored in chameleon with how
// it's going to be played
// b) eliminated the usage of SSH connection, which will be compatible with v3.
// See more at b/262479811 and b/234744284
func PlayFileByPortType(ctx context.Context, chameleond chameleon.Chameleond, token string, portType chameleon.PortType, playbackDuration time.Duration) (err error) {
	ctxCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		if tempErr := chameleond.DeleteFileInChameleon(ctx, token); tempErr != nil {
			err = tempErr
		}
	}(ctxCleanUp)

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
	if err = chameleond.SetUSBDriverPlaybackConfigs(ctx, chameleon.SupportedAudioDataFormat); err != nil {
		return errors.Wrap(err, "failed to set the USB driver playback config")
	}

	if err = chameleond.StartPlayingAudioWithToken(ctx, portID, token, chameleon.SupportedAudioDataFormat); err != nil {
		return errors.Wrap(err, "failed when calling StartPlayingAudioWithToken")
	}
	defer func(ctx context.Context) {
		if tempErr := chameleond.StopPlayingAudio(ctx, portID); tempErr != nil {
			err = tempErr
		}
	}(ctxCleanUp)

	if err = testing.Sleep(ctx, playbackDuration); err != nil {
		return errors.Wrap(err, "failed while sampling: ctx could be expired")
	}

	return err
}
