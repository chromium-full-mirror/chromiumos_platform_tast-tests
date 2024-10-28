// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ehideconst

const (
	// EhidePath is the path for the ehide script.
	EhidePath = "ehide"

	// EhideStateOn is the "on" state for ehide.
	EhideStateOn = "on"

	// EhideStateOff is the "off" state for ehide.
	EhideStateOff = "off"

	// NetNSName is the name of the netns used by ehide.
	NetNSName = "netns-ehide"

	// ResolvConfPath is the path to the resolv.conf file for the physical
	// network.
	ResolvConfPath = "/run/dhclient/etc/resolv.conf"
)
