// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// SetupLoopbackDevice selects the loopback devices as the output and input.
// ALSA loopback (aloop) must be loaded via the fixture.AloopLoaded fixture
// before this function is called.
func SetupLoopbackDevice(ctx context.Context, cr *chrome.Chrome, outDir string, hasError func() bool) (cleanup func(context.Context), err error) {
	timeForCleanUp := 10 * time.Second
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, timeForCleanUp)
	defer cancel()

	cleanup = func(ctx context.Context) {
		// Wait for no stream before unloading aloop as unloading while there is a stream
		// will cause the stream in ARC to be in an invalid state.
		_ = crastestclient.WaitForNoStream(ctx, 5*time.Second)
	}

	if err := audio.SetupLoopback(ctx, cr, outDir, hasError); err != nil {
		crastestclient.DumpAudioDiagnostics(ctx, outDir)
		cleanup(ctxForCleanUp)
		return nil, errors.Wrap(err, "failed to setup loopback")
	}

	return cleanup, nil
}
