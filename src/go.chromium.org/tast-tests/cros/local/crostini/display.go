// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
)

// PrimaryDisplayScaleFactor returns the primary display's scale factor.
func PrimaryDisplayScaleFactor(ctx context.Context, tconn *chrome.TestConn) (factor float64, err error) {
	err = tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.getPrimaryDisplayScaleFactor)()`, &factor)
	return factor, err
}

// VerifyWindowDensities compares the sizes, which should be from
// PollWindowSize() at low and high density. It returns an error if
// something is wrong with the sizes (not just if the high-density
// window is bigger).
func VerifyWindowDensities(ctx context.Context, tconn *chrome.TestConn, sizeHighDensity, sizeLowDensity coords.Size) error {
	if sizeHighDensity.Width > sizeLowDensity.Width || sizeHighDensity.Height > sizeLowDensity.Height {
		return errors.Errorf("app high density size %v greater than low density size %v", sizeHighDensity, sizeLowDensity)
	}

	tabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed getting tablet mode")
	}

	factor, err := PrimaryDisplayScaleFactor(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed getting primary display scale factor")
	}

	if factor != 1.0 && !tabletMode && (sizeHighDensity.Width == sizeLowDensity.Width || sizeHighDensity.Height == sizeLowDensity.Height) {
		return errors.Errorf("app has high density and low density windows with the same size of %v while the scale factor is %v and tablet mode is %v", sizeHighDensity, factor, tabletMode)
	}
	return nil
}
