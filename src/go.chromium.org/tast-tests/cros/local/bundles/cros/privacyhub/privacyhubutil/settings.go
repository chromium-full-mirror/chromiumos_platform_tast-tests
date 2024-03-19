// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package privacyhubutil contains utility functions for privacy hub tests.
package privacyhubutil

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast/core/errors"
)

func hasState(toggleInfo *uiauto.NodeInfo, state state.State) bool {
	if val, present := toggleInfo.State[state]; present {
		return val
	}
	return false
}

func privacyAndSecurityLink() *nodewith.Finder {
	return nodewith.NameStartingWith("Privacy and security").Role("link")
}

func privacyControlsLink() *nodewith.Finder {
	return nodewith.NameStartingWith("Privacy controls")
}

func navigateToToggle(ctx context.Context, tconn *browser.TestConn, toggleLabel string) (*nodewith.Finder, *uiauto.NodeInfo, error) {
	ui := uiauto.New(tconn)
	toggleLabelNode := nodewith.NameStartingWith(toggleLabel).Role(role.ToggleButton)
	// Wait for the toggle to appear.
	if err := uiauto.Combine("Access the toggle in Privacy Hub",
		ui.WaitUntilExists(privacyAndSecurityLink()),
		ui.DoDefault(privacyAndSecurityLink()),
		ui.WaitUntilExists(privacyControlsLink()),
		ui.DoDefault(privacyControlsLink()),
		ui.WaitUntilExists(toggleLabelNode),
	)(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "couldn't access"+toggleLabel+"toggle in Privacy Hub")
	}
	nodeInfo, err := ui.Info(ctx, toggleLabelNode)
	return toggleLabelNode, nodeInfo, err
}

// IsToggleOn checks if a toggle with the given label in Privacy Controls is on.
func IsToggleOn(ctx context.Context, tconn *browser.TestConn, toggleLabel string) (bool, error) {
	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		return false, err
	}
	defer settings.Close(ctx)

	ui := uiauto.New(tconn)

	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(privacyAndSecurityLink())(ctx); err != nil {
		return false, err
	}

	_, toggleInfo, err := navigateToToggle(ctx, tconn, toggleLabel)
	if err != nil {
		return false, errors.Wrap(err, "couldn't access"+toggleLabel+"toggle state")
	}

	pressedAttribute := "aria-pressed"
	val, ok := toggleInfo.HTMLAttributes[pressedAttribute]
	if !ok {
		return false, errors.Errorf("HTML attribute %q missing", pressedAttribute)
	}

	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return false, errors.Errorf("Illegal boolean value for attribute %v: %v", pressedAttribute, val)
	}

	return boolVal, nil
}

// ClickToggle clicks the specified toggle (changing it's state).
func ClickToggle(ctx context.Context, tconn *browser.TestConn, toggleLabel string) error {
	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		return err
	}
	defer settings.Close(ctx)

	var ui *uiauto.Context = uiauto.New(tconn)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(privacyAndSecurityLink())(ctx); err != nil {
		return err
	}
	// Wait for the specified toggle to appear.
	toggleLabelNode, _, err := navigateToToggle(ctx, tconn, toggleLabel)
	if err != nil {
		return errors.Wrap(err, "couldn't access"+toggleLabel+"toggle state")
	}

	// Toggle the specified toggle.
	if err := ui.DoDefault(toggleLabelNode)(ctx); err != nil {
		return errors.Wrap(err, "couldn't click at the "+toggleLabel+" toggle")
	}

	return nil
}

// SetToggleState changes the state of the given toggle to the given state.
func SetToggleState(ctx context.Context, tconn *browser.TestConn, toggleLabel string, enabled bool) error {
	if currentState, err := IsToggleOn(ctx, tconn, toggleLabel); err != nil {
		return err
	} else if currentState == enabled {
		return nil
	}
	return ClickToggle(ctx, tconn, toggleLabel)
}
