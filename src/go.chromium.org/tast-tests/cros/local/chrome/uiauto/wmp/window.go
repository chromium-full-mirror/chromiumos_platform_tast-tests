// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wmp contains utility functions for window resize operations.
package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ResizableArea setups resizable area before performing resizing.
func ResizableArea(ctx context.Context, tconn *chrome.TestConn) (*coords.Rect, error) {
	ui := uiauto.New(tconn)

	rootWindowFinder := nodewith.HasClass("RootWindow-0").Role(role.Window)
	resizeArea, err := ui.Info(ctx, rootWindowFinder)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get root window info")
	}

	shelfInfo, err := ui.Info(ctx, nodewith.Role(role.Toolbar).HasClass("ShelfView"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get shelf info")
	}
	resizeArea.Location.Height -= shelfInfo.Location.Height

	return &resizeArea.Location, nil
}

// StableDrag drags the window and waits for location be stabled.
func StableDrag(tconn *chrome.TestConn, window *nodewith.Finder, srcPt, endPt coords.Point) uiauto.Action {
	const dragDuration = 3 * time.Second
	ui := uiauto.New(tconn)

	return func(ctx context.Context) error {
		if srcPt.Equals(endPt) {
			return nil
		}

		locationBefore, err := ui.Location(ctx, window)
		if err != nil {
			return err
		}

		if err := mouse.Drag(tconn, srcPt, endPt, dragDuration)(ctx); err != nil {
			return err
		}

		// Racing issue exists and causes this function didn't ensure a stable-drag action.
		// Wait for location changes is essential, not just wait for location to be stable.
		return testing.Poll(ctx, func(ctx context.Context) error {
			locationAfter, err := ui.Location(ctx, window)
			if err != nil {
				return testing.PollBreak(err)
			}
			if locationBefore.Equals(*locationAfter) {
				return errors.New("location hasn't changed")
			}
			return nil
		}, &testing.PollOptions{Timeout: 15*time.Second + dragDuration})
	}
}
