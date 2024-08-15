// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SignaturePad defines the methods for interacting with a signature pad.
type SignaturePad interface {
	// GetCanvasBounds returns the bounds of the signature canvas.
	GetCanvasBounds(ctx context.Context) (coords.Rect, error)

	// StartSignature initiates the signature capturing process.
	StartSignature() uiauto.Action

	// SaveSignature saves the current signature with the specified filename.
	SaveSignature(fileName string) uiauto.Action

	// ClearSignature clears the current signature on the pad.
	ClearSignature() uiauto.Action

	// LoadSignature loads a signature from the specified filename.
	LoadSignature(fileName string) uiauto.Action
}

// getSignaturePadCanvasBounds gets the canvas boundaries of the signature pad.
func getSignaturePadCanvasBounds(ctx context.Context, ud *uidetection.Context, tconn *chrome.TestConn, dataPath func(string) string, canvas *uidetection.Finder) (coords.Rect, error) {
	location, err := ud.Location(ctx, canvas)
	if err != nil {
		return coords.Rect{}, errors.Wrap(err, "failed to get location")
	}

	// Get device scale factor to convert location to pixels.
	deviceScaleFactor, err := display.GetDeviceScaleFactor(ctx, tconn,
		func(info *display.Info) bool {
			return info.IsPrimary
		})
	if err != nil {
		return coords.Rect{}, errors.Wrap(err, "failed to get primary display scale factor")
	}

	bounds := coords.ConvertBoundsFromDPToPX(location.Rect, deviceScaleFactor)
	testing.ContextLog(ctx, "Canvas bounds:", bounds)

	return bounds, nil
}
