// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// AddCellularButton is the finder for adding new SIM profiles in Quick Settings.
	AddCellularButton = nodewith.Name("Add new cellular network").Role(role.Button)
)

// NavigateToNetworkDetailedView will navigate to the detailed Network view
// within the Quick Settings. This is safe to call even when the Quick Settings
// are already open.
func NavigateToNetworkDetailedView(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	qsRevampEnabled, err := QsRevampEnabled(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get QsRevamp state")
	}
	networkDetailedView, err := NetworkDetailedView(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get network detailed view")
	}
	// The quicksettings could be collapsed during navigating to the certain view,
	// typically caused by pop-up window, notifications or other display rendering event, retrying it is essential.
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := Expand(ctx, tconn); err != nil {
			return err
		}

		// The network item depends on whether QsRevamp is enabled or not.
		var networkItem *nodewith.Finder
		if qsRevampEnabled {
			networkItem = FeatureTileNetwork
		} else {
			networkItem = nodewith.HasClass("FeaturePodLabelButton").NameContaining("network").Ancestor(LegacyRootFinder)
		}
		return uiauto.Combine("click the Network item in quick setttings",
			ui.WithTimeout(5*time.Second).LeftClick(networkItem),
			ui.WithTimeout(5*time.Second).WaitUntilExists(networkDetailedView),
		)(ctx)
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second})
}

// OpenNetworkSettings will open the Network settings within the Quick Settings.
// NavigateToNetworkDetailedView() must be called in advance.
func OpenNetworkSettings(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	quickSettingsRoot, err := GetRootFinder(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get quick settings root")
	}
	networkDetailedView, err := NetworkDetailedView(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get network detailed view")
	}
	networkSettingsButton := nodewith.HasClass("IconButton").Name("Network settings").Ancestor(quickSettingsRoot)
	return uiauto.Combine("click the Network settings",
		ui.LeftClick(networkSettingsButton),
		ui.WaitUntilGone(networkDetailedView),
	)(ctx)
}

// NetworkDetailedView returns the detailed Network view within Quick Settings.
func NetworkDetailedView(ctx context.Context, tconn *chrome.TestConn) (*nodewith.Finder, error) {
	quickSettingsRoot, err := GetRootFinder(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get quick settings root")
	}
	return nodewith.HasClass("NetworkDetailedNetworkViewImpl").Ancestor(quickSettingsRoot), nil
}

// NetworkListItemView returns the network item list on the network view in Quick Settings.
func NetworkListItemView(ctx context.Context, tconn *chrome.TestConn) (*nodewith.Finder, error) {
	quickSettingsRoot, err := GetRootFinder(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get quick settings root")
	}
	return nodewith.HasClass("NetworkListNetworkItemView").Ancestor(quickSettingsRoot), nil
}

// NetworkDetailedViewWifiToggleButton returns the WiFi toggle within the Network detailed view.
func NetworkDetailedViewWifiToggleButton(ctx context.Context, tconn *chrome.TestConn) (*nodewith.Finder, error) {
	qsRevampEnabled, err := QsRevampEnabled(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get QsRevamp state")
	}
	networkDetailedView, err := NetworkDetailedView(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network detailed view")
	}
	// Legacy quick settings uses a TrayToggleButton.
	if !qsRevampEnabled {
		return nodewith.HasClass("TrayToggleButton").NameContaining("Wi-Fi").Ancestor(networkDetailedView), nil
	}
	// QsRevamp uses an ordinary button.
	return nodewith.Role(role.Button).NameContaining("Toggle Wi-Fi").Ancestor(networkDetailedView), nil
}

// NetworkDetailedViewMobileDataToggle returns the switch to enable/disable Mobile data within network quick settings.
func NetworkDetailedViewMobileDataToggle(ctx context.Context, tconn *chrome.TestConn) (*nodewith.Finder, error) {
	qsRevampEnabled, err := QsRevampEnabled(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get QsRevamp state")
	}
	networkDetailedView, err := NetworkDetailedView(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network detailed view")
	}
	// Legacy quick settings uses a TrayToggleButton.
	if !qsRevampEnabled {
		return nodewith.Name("Mobile data").HasClass("TrayToggleButton").Ancestor(networkDetailedView), nil
	}
	// QsRevamp uses an ordinary button.
	return nodewith.Role(role.Button).NameContaining("Toggle mobile data").Ancestor(networkDetailedView), nil
}
