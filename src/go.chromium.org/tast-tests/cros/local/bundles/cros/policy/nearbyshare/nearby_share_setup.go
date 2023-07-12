// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package nearbyshare contains helpers to verify NearbyShare policies.
package nearbyshare

import (
	"context"
	"net/http/httptest"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// TestCase defines test expectations based on the value of policy
// NearbyShareAllowed.
type TestCase struct {
	Name             string
	ShouldBeDisabled bool
	Policy           *policy.NearbyShareAllowed
}

// GetTestCases returns the list of TestCase objects on which
// NearbyShareAllowed policy is tested.
func GetTestCases() []TestCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return []TestCase{
		{
			Name:             "disabled",
			ShouldBeDisabled: true,
			Policy:           &policy.NearbyShareAllowed{Val: false},
		},
		{
			Name:             "enabled",
			ShouldBeDisabled: false,
			Policy:           &policy.NearbyShareAllowed{Val: true},
		},
	}
}

// VerifyNearbySharePermissions opens the 'Connected devices' page in OS
// Settings and verifies that Nearby Share is disabled based on policy value.
func VerifyNearbySharePermissions(ctx context.Context, cr *chrome.Chrome, _ *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, paramIndex int) (err error) {
	param := GetTestCases()[paramIndex]

	const (
		connectedDevicesURL      = "multidevice"
		connectedDevicesPageName = "Connected devices"
		setupButtonName          = "Set up"
		toggleName               = "Nearby Share"
	)

	// Open 'Connected devices' page in OS Settings.
	ui := uiauto.New(tconn)
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, connectedDevicesURL,
		ui.WaitUntilExists(nodewith.Name(connectedDevicesPageName).First()))
	if err != nil {
		return errors.Wrap(err, "failed to launch Connected devices OS Settings page")
	}

	if param.ShouldBeDisabled {
		// Verify toggle is set to disabled.
		if isEnabled, err := settings.IsToggleOptionEnabled(ctx, cr, toggleName); err != nil {
			return errors.Wrap(err, "failed to get current toggle value")
		} else if isEnabled {
			return errors.Wrap(err, "toggle is enabled when it should be disabled")
		}

		// Verify toggle is restricted (cannot be changed).
		nearbyShareToggle := nodewith.Name(toggleName).Role(role.ToggleButton)
		info, err := ui.Info(ctx, nearbyShareToggle)
		if err != nil {
			return errors.Wrap(err, "failed to get info about nearby share toggle")
		}
		if info.Restriction != restriction.Disabled {
			return errors.Wrap(err, "nearby share toggle is not restricted")
		}
	} else {
		// Verify 'Set up' button is displayed.
		nearbyShareSetupButton := nodewith.Name(setupButtonName).Role(role.Button)
		if err := ui.WaitUntilExists(nearbyShareSetupButton)(ctx); err != nil {
			return errors.Wrap(err, "nearby share 'Set up' button missing")
		}
	}
	return nil
}

func selectorFinder(name, class string) *nodewith.Finder {
	return nodewith.Role(role.ListBoxOption).Name(name).HasClass(class)
}
