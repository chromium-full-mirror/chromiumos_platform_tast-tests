// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hotspotutil

import (
	"go.chromium.org/tast-tests/cros/services/cros/ui"
)

// OneDeviceConnectedMessage is the finder of the "1 device connected" message in hotspot detailed view.
var OneDeviceConnectedMessage = &ui.Finder{
	NodeWiths: []*ui.NodeWith{
		{Value: &ui.NodeWith_Name{Name: "1 device connected"}},
		{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
	},
}

// NoDeviceConnectedMessage is the finder of the "No devices connected" message in hotspot detailed view.
var NoDeviceConnectedMessage = &ui.Finder{
	NodeWiths: []*ui.NodeWith{
		{Value: &ui.NodeWith_Name{Name: "No devices connected"}},
		{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
	},
}

// HotspotSwitch is the finder of the hotspot toggle in hotspot detailed view.
var HotspotSwitch = &ui.Finder{
	NodeWiths: []*ui.NodeWith{
		{Value: &ui.NodeWith_Name{Name: "Toggle hotspot"}},
		{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_SWITCH}},
	},
}
