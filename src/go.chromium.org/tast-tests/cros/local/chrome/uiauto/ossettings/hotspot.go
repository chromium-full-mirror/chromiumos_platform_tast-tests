// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
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

	// Specify the ancestor, since elements in main settings page might have the same attribute with elements in the navigation list.
	mainPage := nodewith.Role(role.Main).Ancestor(WindowFinder)
	if err := s.ui.WaitUntilExists(HotspotOffSublabel.Ancestor(mainPage))(ctx); err != nil {
		return errors.Wrap(err, "failed to find expected hotspot status label")
	}

	return nil
}

// RenameHotspotSsid renames the hotspot ssid to newSsid in hotspot config dialog
func (s *OSSettings) RenameHotspotSsid(ctx context.Context, cr *chrome.Chrome, newSsid string) uiauto.Action {
	return func(ctx context.Context) error {
		if err := uiauto.Combine("opening the hotspot configuration dialog",
			s.WaitUntilExists(HotspotHeading),             // Ensuring the settings app is on the "Hotspot" page.
			s.LeftClick(HotspotConfigureButton),           // Click the button to launch the dialog.
			s.WaitUntilExists(HotspotConfigurationDialog), // Ensuring the dialog is opened.
		)(ctx); err != nil {
			return err
		}

		expr := fmt.Sprintf(`
			var inputs = shadowPiercingQueryAll('input[aria-label="Hotspot name"]');
			if (inputs.length != 1) { throw new Error("couldn't find the unique hotspot input field"); }
			inputs[0].value = %q;
			inputs[0].dispatchEvent(new Event('input'));
		`, newSsid)

		// TODO(b/352439932): Revert the JS approach back to the UI approach once the issue is fixed.
		// The body of the Hotspot Configuration dialog is not loaded onto the a11y tree,
		// hence, the entire body can't be interacted with through the automation API.
		//
		// Note that updating elements triggers the a11y tree to be refreshed (partially),
		// allowing those elements to be interacted with through the automation API.
		// However, since we are invoking JS call, we might as well do the rename by using JS.
		if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, nil); err != nil {
			return errors.Wrap(err, "failed to change the value of input field")
		}

		return s.LeftClick(nodewith.Name("Save").Role(role.Button))(ctx)
	}
}
