// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dropdown provides utilities to interact with dropdowns.
package dropdown

import (
	"context"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// Values return all available choices for the given dropdown.
// Expects that the given dropdown is closed.
func Values(ctx context.Context, tconn *chrome.TestConn, dropdown *nodewith.Finder) ([]string, error) {
	ui := uiauto.New(tconn)
	// Click on the dropdown and wait for it to expand.
	if err := uiauto.Combine("open dropdown",
		ui.DoDefault(dropdown),
		ui.WaitUntilExists(dropdown.State("expanded", true)))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to expand dropdown")
	}

	// Search for all available values nodes.
	valueNodes, err := ui.NodesInfo(ctx, nodewith.Ancestor(dropdown).Role(role.ListBoxOption))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch available dropdown values")
	}
	availableValues := make([]string, 0)
	for _, availableValueNode := range valueNodes {
		availableValues = append(availableValues, availableValueNode.Name)
	}

	// Click on the dropdown and wait for it to collapse.
	// Otherwise the expanded dropdown may break some future UI interactions.
	if err := uiauto.Combine("close dropdown",
		ui.DoDefault(dropdown),
		ui.WaitUntilExists(dropdown.State("expanded", false)))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to close dropdown")
	}

	return availableValues, nil
}
