// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

// CitrixData holds the UI fragments that are used by Citrix connector. Use
// this as a data dependency when connecting to Citrix.
var CitrixData = []string{
	"citrix/Splashscreen_ServerUrlTbx.png",
}

const (
	topBtnIcon           = "citrix/top_btn.png"
	usbDeviceBtnIcon     = "citrix/usb_device_btn.png"
	closeDialogBtnIcon   = "citrix/close_dialog_btn.png"
	moreOptionBtnIcon    = "citrix/more_option_btn.png"
	resolutionButtonIcon = "citrix/resolution_btn.png"
)

// UIFragmentName is the identifier to retrieve location of image from
// UiFragments.
type UIFragmentName int

const (
	// SplashscreenServerURLTbx is an id for retrieving path to the image.
	SplashscreenServerURLTbx UIFragmentName = iota
)

type citrixResolution string

const (
	// ResolutionAutoFitScreen is the resolution option "Auto-fit screen".
	ResolutionAutoFitScreen citrixResolution = "Auto-fit screen"
	// ResolutionDevicePixelRatioScaling is the resolution option "Device Pixel Ratio Scaling".
	ResolutionDevicePixelRatioScaling citrixResolution = "Device Pixel Ratio Scaling"
	// Resolution1280 is the resolution option "1280 x 800 pixels".
	Resolution1280 citrixResolution = "1280 x 800 pixels"
	// Resolution1440 is the resolution option "1440 x 900 pixels".
	Resolution1440 citrixResolution = "1440 x 900 pixels"
	// Resolution1600 is the resolution option "1600 x 1200 pixels".
	Resolution1600 citrixResolution = "1600 x 1200 pixels"
)

var citrixResolutionSlice = []citrixResolution{
	ResolutionAutoFitScreen,
	ResolutionDevicePixelRatioScaling,
	Resolution1280,
	Resolution1440,
	Resolution1600,
}
