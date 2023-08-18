// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio/internal"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LoadAloop is deprecated.
//
// Deprecated: Use the fixture.AloopLoaded() instead.
var LoadAloop = internal.LoadAloop

// SetupLoopback selects the playback and capture nodes to the ALSA loopback via the Quick Settings UI.
func SetupLoopback(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

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
	if err := quicksettings.SelectAudioOption(ctx, tconn, "Loopback Playback"); err != nil {
		return errors.Wrap(err, "failed to select ALSA loopback output")
	}

	// After selecting Loopback Playback, SelectAudioOption() sometimes detected that audio setting
	// is still opened while it is actually fading out, and failed to select Loopback Capture.
	// Call Hide() and Show() to reset the quicksettings menu first.
	if err := quicksettings.Hide(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to hide the quicksettings before show")
	}
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to show the quicksettings to select capture node")
	}
	if err := quicksettings.SelectAudioOption(ctx, tconn, "Loopback Capture"); err != nil {
		return errors.Wrap(err, "failed to select ALSA loopback input")
	}

	return nil
}
