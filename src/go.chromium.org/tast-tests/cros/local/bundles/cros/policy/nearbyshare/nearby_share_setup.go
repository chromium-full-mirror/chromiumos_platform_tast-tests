// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package nearbyshare contains helpers to verify NearbyShare policies.
package nearbyshare

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
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

// TestCases returns the map of policy settings enum to TestCase
// objects on which the NearbyShareAllowed policy is tested.
func TestCases() map[networkrequestmonitor.PolicySetting]TestCase {
	return map[networkrequestmonitor.PolicySetting]TestCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:             "disabled",
			ShouldBeDisabled: true,
			Policy:           &policy.NearbyShareAllowed{Val: false},
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:             "enabled",
			ShouldBeDisabled: false,
			Policy:           &policy.NearbyShareAllowed{Val: true},
		},
	}
}

// VerifyNearbySharePermissions opens the 'Connected devices' page in OS
// Settings and verifies that Nearby Share is disabled based on policy value.
func VerifyNearbySharePermissions(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	policySetting := params.PolicySetting
	cr := params.Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}
	policyParam := TestCases()[policySetting]

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

	if policyParam.ShouldBeDisabled {
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
