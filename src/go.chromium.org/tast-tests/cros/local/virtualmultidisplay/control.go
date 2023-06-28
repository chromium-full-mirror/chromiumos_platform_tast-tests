// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package virtualmultidisplay

import (
	"context"
)

// VirtualDisplayController provides functions to control displays.
// In order to use this interface, the multiDisplay fixture (or a child of it)
// *must* be loaded.
type VirtualDisplayController interface {
	// DisplayEnabled returns whether the given display is enabled.  Returns an
	// error if display id out of range.
	DisplayEnabled(displayID int) (bool, error)

	// EnableDisplay enables a given display, returns an error when the display id
	// is above max displays.
	EnableDisplay(displayID int) error

	// DisableDisplay disables a given display, returns an error when the display
	// id is above max displays.
	DisableDisplay(displayID int) error

	// DisplayCount returns the number of available virtual displays.
	DisplayCount() (int, error)

	// InternalDisplayId (by index) returns which of the display is the built in,
	// internal one.
	InternalDisplayID() (int, error)

	// ExternalDisplayIDs (by index) returns which of the displays are not the
	// built in, internal one.
	ExternalDisplayIDs() ([]int, error)

	// AdditionalFixtureSetup does some backend specific setup (such as mount
	// configfs for VKMS).
	AdditionalFixtureSetup(ctx context.Context) error

	// AdditionalFixtureTeardown does some backend specific teardown (such as
	// rmdir all configfs for VKMS).
	AdditionalFixtureTeardown(ctx context.Context) error
}
