// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/audio/internal"
	"go.chromium.org/tast-tests/cros/local/chrome"
)

// LoadAloop is deprecated.
//
// Deprecated: Use the fixture.AloopLoaded() instead.
var LoadAloop = internal.LoadAloop

// SetupLoopback selects the playback and capture nodes to the ALSA loopback via the Quick Settings UI.
func SetupLoopback(ctx context.Context, cr *chrome.Chrome, outDir string, hasError func() bool) error {
	return SelectDevicesViaQuickSettings(ctx, cr, outDir, hasError, "Loopback Playback", "Loopback Capture")
}
