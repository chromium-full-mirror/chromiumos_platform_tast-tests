// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package virtualmultidisplay

// VirtualDisplayController provides functions to control displays.
// In order to use this interface, the multiDisplay fixture (or a child of it) *must* be loaded.
type VirtualDisplayController interface {
	// DisplayEnabled returns whether the given display is enabled.  Returns an error if display id out of range.
	DisplayEnabled(displayID int) (bool, error)

	// EnableDisplay enables a given display, returns an error when the display id is above max displays.
	EnableDisplay(displayID int) error

	// DisableDisplay disables a given display, returns an error when the display id is above max displays.
	DisableDisplay(displayID int) error

	// DisplayCount returns the number of available virtual displays.
	DisplayCount() (int, error)

	// InternalDisplayId returns which of the display is the built in, internal one.
	InternalDisplayId() (int, error)

	// ExternalDisplayIds returns which of the displays are not the built in, internal one.
	ExternalDisplayIds() ([]int, error)
}
