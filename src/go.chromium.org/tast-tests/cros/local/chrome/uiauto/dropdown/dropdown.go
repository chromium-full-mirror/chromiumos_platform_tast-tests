// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dropdown provides utilities to interact with dropdowns.
package dropdown

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// getValueNodeInfos retrieves the NodeInfo for all options of a dropdown. It
// also ensures the dropdown is closed before completing to avoid side effects.
func getValueNodeInfos(ctx context.Context, tconn *chrome.TestConn, dropdown *nodewith.Finder) ([]uiauto.NodeInfo, error) {
	ui := uiauto.New(tconn)
	// Click on the dropdown and wait for it to expand.
	if err := uiauto.Combine("open dropdown",
		ui.DoDefault(dropdown),
		ui.WaitUntilExists(dropdown.State("expanded", true)))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to expand dropdown")
	}

	// Search for all available option nodes.
	valueNodes, err := ui.NodesInfo(ctx, nodewith.Ancestor(dropdown).Role(role.ListBoxOption))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch available option nodes")
	}

	// Click on the dropdown and wait for it to collapse.
	// Otherwise the expanded dropdown may break some future UI interactions.
	if err := uiauto.Combine("close dropdown",
		ui.DoDefault(dropdown),
		ui.WaitUntilExists(dropdown.State("expanded", false)))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to close dropdown")
	}

	return valueNodes, nil
}

// Values return all available choices for the given dropdown.
// Expects that the given dropdown is closed.
func Values(ctx context.Context, tconn *chrome.TestConn, dropdown *nodewith.Finder) ([]string, error) {
	// Search for all available values nodes.
	valueNodes, err := getValueNodeInfos(ctx, tconn, dropdown)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch available dropdown values")
	}
	availableValues := make([]string, 0)
	for _, availableValueNode := range valueNodes {
		availableValues = append(availableValues, availableValueNode.Name)
	}
	return availableValues, nil
}

// SelectDropDownOption returns a function that selects dropdown option with the given option name.
func SelectDropDownOption(tconn *chrome.TestConn, dropdown *nodewith.Finder, optionName string) uiauto.Action {
	ui := uiauto.New(tconn)
	option := nodewith.Name(optionName).Role(role.ListBoxOption).Ancestor(dropdown)
	return uiauto.Combine(fmt.Sprintf("select option %q", optionName),
		ui.WaitUntilExists(dropdown),
		ui.MakeVisible(dropdown),
		ui.LeftClickUntil(dropdown, ui.Exists(option)),
		ui.LeftClickUntil(option, ui.Gone(option)),
	)
}

// IsSelected returns true if the label matches the selected option, otherwise
// returns false.
func IsSelected(ctx context.Context, tconn *chrome.TestConn, dropdown *nodewith.Finder, optionName string) (bool, error) {
	// Search for all available values nodes.
	valueNodes, err := getValueNodeInfos(ctx, tconn, dropdown)
	if err != nil {
		return false, errors.Wrap(err, "failed to fetch available dropdown values")
	}
	for _, availableValueNode := range valueNodes {
		if availableValueNode.Selected && availableValueNode.Name == optionName {
			return true, nil
		}
	}
	return false, nil
}
