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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LaunchAtHotspotSubpage launch OS settings and navigate to Hotspot subpage.
func LaunchAtHotspotSubpage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*OSSettings, error) {
	condition := uiauto.New(tconn).Exists(nodewith.Name("Hotspot subpage back button"))
	return LaunchAtPageURL(ctx, tconn, cr, "hotspotDetail", condition)
}

// ToggleHotspot toggles on/off hotspot and verify its status changes to expected.
// It does nothing if the hotspot status is already expected.
func (s *OSSettings) ToggleHotspot(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, expected bool) error {
	const toggleName = "Hotspot"
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
		return errors.Wrap(err, "failed to set toggle option as expected")
	}

	if expected {
		const notificationTitle = "Hotspot is on (Wi-Fi is off)"
		// Logging all notifications found for further troubleshooting,
		// as the hotspot appears to have automatically turned off somehow.
		// TODO(b/351070946): Remove the logs once the issue has been identified or got fixed.
		predicate := func(n *ash.Notification) bool {
			testing.ContextLog(ctx, "Notification title: ", n.Title)
			testing.ContextLog(ctx, "Notification message: ", n.Message)
			return n.Title == notificationTitle
		}
		if _, err := ash.WaitForNotification(ctx, tconn, time.Minute, predicate); err != nil {
			return errors.Wrap(err, "failed to wait for notification with title: Hotspot is on (Wi-Fi is off)")
		}

		// Close all notifications.
		if err := ash.CloseNotifications(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all notifications after toggling hotspot")
		}
		return nil
	}

	if err := s.ui.WaitUntilExists(HotspotOffSublabel)(ctx); err != nil {
		return errors.Wrap(err, "failed to find expected hotspot status label")
	}

	return nil
}

// RenameHotspotSsid renames the hotspot ssid to newSsid in hotspot config dialog
func (s *OSSettings) RenameHotspotSsid(ctx context.Context, newSsid string) uiauto.Action {
	return func(ctx context.Context) error {
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard")
		}
		defer kb.Close(ctx)

		return uiauto.Combine("Rename hotspot ssid in config dialog",
			s.ui.LeftClick(HotspotConfigureButton),
			s.ui.LeftClick(HotspotNameTextField),
			kb.AccelAction("Ctrl+A"),
			kb.AccelAction("Backspace"),
			kb.TypeAction(newSsid),
			s.ui.LeftClick(nodewith.Name("Save").Role(role.Button)),
		)(ctx)
	}
}
