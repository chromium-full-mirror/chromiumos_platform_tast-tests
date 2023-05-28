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
	"go.chromium.org/tast/core/testing"
)

var (
	// NetworkDetailedView is the detailed Network view within Quick Settings.
	NetworkDetailedView = nodewith.HasClass("NetworkDetailedNetworkViewImpl").Ancestor(RootFinder)

	// NetworkListItemView is the network item list on the network view in Quick Settings.
	NetworkListItemView = nodewith.HasClass("NetworkListNetworkItemView").Ancestor(RootFinder)

	// NetworkFeaturePodLabelButton is the label child of the Network feature pod button.
	NetworkFeaturePodLabelButton = nodewith.HasClass("FeaturePodLabelButton").NameContaining("network").Ancestor(RootFinder)

	// networkSettingsButton is the button shown on the Network detailed view.
	networkSettingsButton = nodewith.HasClass("IconButton").Name("Network settings").Ancestor(RootFinder)

	// NetworkDetailedViewWifiToggleButton is the WiFi toggle within the Network detailed view.
	NetworkDetailedViewWifiToggleButton = nodewith.HasClass("TrayToggleButton").NameContaining("Wi-Fi").Ancestor(NetworkDetailedView)

	// NetworkDetailedViewMobileDataToggle is the switch to enable/disable Mobile data within network quick settings
	NetworkDetailedViewMobileDataToggle = nodewith.Name("Mobile data").HasClass("TrayToggleButton").Ancestor(NetworkDetailedView)

	// AddCellularButton is the finder for adding new SIM profiles in Quick Settings.
	AddCellularButton = nodewith.Name("Add new cellular network").Role(role.Button)
)

// NavigateToNetworkDetailedView will navigate to the detailed Network view
// within the Quick Settings. This is safe to call even when the Quick Settings
// are already open.
func NavigateToNetworkDetailedView(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	// The quicksettings could be collapsed during navigating to the certain view,
	// typically caused by pop-up window, notifications or other display rendering event, retrying it is essential.
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := Expand(ctx, tconn); err != nil {
			return err
		}

		return uiauto.Combine("click the Network feature pod label",
			ui.WithTimeout(5*time.Second).LeftClick(NetworkFeaturePodLabelButton),
			ui.WithTimeout(5*time.Second).WaitUntilExists(NetworkDetailedView),
		)(ctx)
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second})
}

// OpenNetworkSettings will open the Network settings within the Quick Settings.
// NavigateToNetworkDetailedView() must be called in advance.
func OpenNetworkSettings(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	return uiauto.Combine("click the Network settings",
		ui.LeftClick(networkSettingsButton),
		ui.WaitUntilGone(NetworkDetailedView),
	)(ctx)
}
