// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package citrix holds the implementation of the /vdi/apps/vdiconnector.VDIInt
// interface for Citrix application.
package citrix

// AppName is the name of the Citrix application.
type AppName string

const (
	// FootPedalAppName is the name of the Foot Pedal application.
	FootPedalAppName AppName = "Philips"
	// ScriptelAppName is the name of the Scriptel application.
	ScriptelAppName AppName = "Scrip"
	// TopazAppName is the name of the Topaz application.
	TopazAppName AppName = "DemoOCX32"
)

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

// SignaturePadData holds the UI fragments that are used by Topaz/Scriptel signature. Use
// this as a data dependency when connecting to Topaz/Scriptel.
var SignaturePadData = []string{
	ScriptelAppIcon,
	scriptelTransparentIcon,
	scriptelClearIcon,
	topazClearIcon,
}

const (
	// ScriptelMotionData is the motion data that controls the robotic arm to
	// operate the Scriptel signature pad.
	ScriptelMotionData = "scriptel_motion_data.csv"
	// ScriptelAppTitle is the app title of the Scriptel app.
	ScriptelAppTitle = "ScripTouch Sign and Save"
	// ScriptelAppIcon is the file name of the Scriptel app icon.
	ScriptelAppIcon         = "scriptel_icon.png"
	scriptelTransparentIcon = "scriptel_transparent.png"
	scriptelClearIcon       = "scriptel_clear.png"
)

const (
	// TopazMotionData is the motion data that controls the robotic arm to
	// operate the Topaz signature pad.
	TopazMotionData = "topaz_motion_data.csv"
	// TopazAppTitle is the app title of the Topaz app.
	TopazAppTitle = "Topaz SigPlus Demonstration"
	// TopazDeviceName is the device name of the Topaz signature pad.
	TopazDeviceName = "Topaz HID Tablet"
	topazClearIcon  = "topaz_clear.png"
)
