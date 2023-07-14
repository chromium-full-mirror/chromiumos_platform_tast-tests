// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/testing"
)

// hotspotDetailedView is the detailed Hotspot view within the Quick
// Settings.
var hotspotDetailedView = nodewith.ClassName("HotspotDetailedView")

// NavigateToHotspotDetailedView will navigate to the detailed Hotspot view
// within the Quick Settings. This is safe to call even when the Quick Settings
// are already open.
func NavigateToHotspotDetailedView(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	// The quicksettings could be collapsed during navigating to the certain view,
	// typically caused by pop-up window, notifications or other display rendering event, retrying it is essential.
	return testing.Poll(ctx, func(ctx context.Context) error {
		// Open quick settings, since checking NearbyShareEnabled() needs it open.
		// This avoids opening and closing quick settings twice.
		cleanup, err := ensureVisible(ctx, tconn)
		if err != nil {
			return err
		}
		defer cleanup(ctx)

		return uiauto.Combine("click the Hotspot tile in quick setttings",
			ui.WithTimeout(5*time.Second).LeftClick(FeatureTileHotspot),
			ui.WithTimeout(5*time.Second).WaitUntilExists(hotspotDetailedView),
		)(ctx)
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second})
}

// IsHotspotTileShown returns whether the Hotspot feature tile is shown in
// quick settings.
func IsHotspotTileShown(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return false, err
	}
	defer cleanup(ctx)

	ui := uiauto.New(tconn)
	return ui.IsNodeFound(ctx, FeatureTileHotspot)
}
