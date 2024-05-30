// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SelectIODevices sets the default input and output devices of CRAS.
//
// UI must be stopped otherwise it may overwrite the settings.
func SelectIODevices(ctx context.Context, cras *Cras, defaultInputDeviceType, defaultOutputDeviceType string) error {
	testing.ContextLog(ctx, "Setting default input device to ", defaultInputDeviceType)
	if err := cras.SetActiveNodeByMatcher(ctx, MatchNodeTypeDirection{
		Type:      defaultInputDeviceType,
		Direction: InputStream,
	}); err != nil {
		return errors.Wrapf(err, "cannot set default input device to %s", defaultInputDeviceType)
	}
	testing.ContextLog(ctx, "Setting default output device to ", defaultOutputDeviceType)
	if err := cras.SetActiveNodeByMatcher(ctx, MatchNodeTypeDirection{
		Type:      defaultOutputDeviceType,
		Direction: OutputStream,
	}); err != nil {
		return errors.Wrapf(err, "cannot set default output device to %s", defaultOutputDeviceType)
	}
	return nil
}

// SelectDevicesViaQuickSettings selects the default input (and output) devices of CRAS via the Quick Settings UI.
func SelectDevicesViaQuickSettings(ctx context.Context, cr *chrome.Chrome, outDir string, hasError func() bool, devices ...string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	defer faillog.DumpUITreeOnError(ctx, outDir, hasError, tconn)

	timeForCleanUp := 5 * time.Second
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, timeForCleanUp)
	defer cancel()

	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to show the quicksettings to select playback node")
	}
	defer func() {
		if err := quicksettings.Hide(ctxForCleanUp, tconn); err != nil {
			testing.ContextLog(ctx, "Failed to hide the quicksettings on defer: ", err)
		}
	}()
	for i, device := range devices {
		if i > 0 {
			// After selecting a device, SelectAudioOption() sometimes detected that audio setting
			// is still opened while it is actually fading out, and failed to select Loopback Capture.
			// Call Hide() and Show() to reset the quicksettings menu first.
			if err := quicksettings.Hide(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to hide the quicksettings before show")
			}
			if err := quicksettings.Show(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to show the quicksettings to select capture node")
			}
		}
		if err := quicksettings.SelectAudioOption(ctx, tconn, device); err != nil {
			return errors.Wrap(err, "failed to select ALSA loopback output")
		}
	}
	return nil
}
