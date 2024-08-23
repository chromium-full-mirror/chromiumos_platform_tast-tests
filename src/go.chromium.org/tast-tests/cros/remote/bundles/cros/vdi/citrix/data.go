// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package citrix holds the implementation of the /vdi/apps/vdiconnector.VDIInt
// interface for Citrix application.
package citrix

// AppName is the name of the Citrix application.
type AppName string

// FootPedalAppName is the name of the Foot Pedal application.
const FootPedalAppName AppName = "Philips"

const (
	topBtnIcon           = "citrix/top_btn.png"
	usbDeviceBtnIcon     = "citrix/usb_device_btn.png"
	closeDialogBtnIcon   = "citrix/close_dialog_btn.png"
	moreOptionBtnIcon    = "citrix/more_option_btn.png"
	resolutionButtonIcon = "citrix/resolution_btn.png"
)

// CitrixData holds the UI fragments that are used by Citrix connector. Use
// this as a data dependency when connecting to Citrix.
var CitrixData = []string{
	topBtnIcon,
	usbDeviceBtnIcon,
	closeDialogBtnIcon,
	moreOptionBtnIcon,
	resolutionButtonIcon,
}

// FootPedalData holds the UI fragments that are used by Foot Pedal. Use
// this as a data dependency when connecting to Foot Pedal.
var FootPedalData = []string{
	footPedalMarkIcon,
	footPedalMaximizeIcon,
}

const (
	footPedalMarkIcon     = "foot_pedal_mark.png"
	footPedalMaximizeIcon = "foot_pedal_maximize.png"
)
