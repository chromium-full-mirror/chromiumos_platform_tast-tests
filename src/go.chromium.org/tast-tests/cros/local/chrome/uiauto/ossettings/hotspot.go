// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/errors"
)

// LaunchAtHotspotSubpage launch OS settings and navigate to Hotspot subpage.
func LaunchAtHotspotSubpage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*OSSettings, error) {
	condition := uiauto.New(tconn).Exists(nodewith.Name("Hotspot subpage back button"))
	return LaunchAtPageURL(ctx, tconn, cr, "hotspotDetail", condition)
}

// ToggleHotspot toggles on/off hotspot and verify its status changes to expected.
// It does nothing if the hotspot status is already expected.
func (s *OSSettings) ToggleHotspot(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, expected bool) error {
	const toggleName = "Hotspot enable"
	if enabled, err := s.IsToggleOptionEnabled(ctx, cr, toggleName); err != nil {
		return errors.Wrap(err, "faled to check option enabled")
	} else if enabled == expected {
		return nil
	}

	// Close all notifications before toggling.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to close all notifications before toggle hotspot")
	}

	if err := s.SetToggleOption(cr, toggleName, expected)(ctx); err != nil {
		return errors.Wrapf(err, "failed to toggle hotspot to %t", expected)
	}

	statusLabel := HotspotOffSublabel
	if expected {
		statusLabel = HotspotOnSublabel
		const notificationTitle = "Hotspot is on"
		if _, err := ash.WaitForNotification(ctx, tconn, time.Minute, ash.WaitTitle(notificationTitle)); err != nil {
			return errors.Wrap(err, "failed to wait for notification with title: Hotspot is on")
		}

		// Close all notifications.
		if err := ash.CloseNotifications(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all notifications after toggling hotspot")
		}
	}

	if err := s.ui.WaitUntilExists(statusLabel)(ctx); err != nil {
		return errors.Wrap(err, "failed to find expected hotspot status label")
	}

	return nil
}
