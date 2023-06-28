// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"regexp"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// LegacyRootFinder is the finder to find the Quick Settings area in the UI.
// It should be used when feature QsRevamp is disabled.
var LegacyRootFinder = nodewith.HasClass("UnifiedSystemTrayView")

// QsRootFinder is the finder to find the Quick Settings area in the UI.
// It should be used when feature QsRevamp is enabled.
var QsRootFinder = nodewith.HasClass("QuickSettingsView")

// SystemTray is to distinguish it from calendar view in cases that use the calendar, it is also the finder to find the Quick Settings area in the UI.
var SystemTray = nodewith.HasClass("UnifiedSystemTray")

// StatusAreaWidget is the finder to find the control widgets.
var StatusAreaWidget = nodewith.Role(role.Pane).HasClass("ash/StatusAreaWidgetDelegate")

// FeatureTileAccessibility is the finder for the "Accessibility" feature tile.
// It only exists with QsRevamp.
var FeatureTileAccessibility = nodewith.HasClass("FeatureTile").NameContaining("accessibility")

// FeatureTileBluetooth is the finder for the "Bluetooth" feature tile.
// It only exists with QsRevamp.
var FeatureTileBluetooth = nodewith.HasClass("FeatureTile").NameContaining("Bluetooth")

// FeatureTileBluetoothToggle is the finder for the toggle button that is part
// of the Bluetooth feature tile. It only exists with QsRevamp.
var FeatureTileBluetoothToggle = nodewith.Role(role.Button).NameContaining("Toggle Bluetooth").Ancestor(FeatureTileBluetooth)

// FeatureTileCast is the finder for the "Cast screen" feature tile.
// It only exists with QsRevamp.
var FeatureTileCast = nodewith.HasClass("FeatureTile").NameContaining("cast")

// FeatureTileDoNotDisturb is the finder for the "Do not disturb" feature tile.
// It only exists with QsRevamp.
var FeatureTileDoNotDisturb = nodewith.HasClass("FeatureTile").NameContaining("Do not disturb")

// FeatureTileKeyboard is the finder for the "Keyboard" (IME) feature tile.
// It only exists with QsRevamp.
var FeatureTileKeyboard = nodewith.HasClass("FeatureTile").NameContaining("keyboard")

// FeatureTileNetwork is the finder for the network feature tile. Its name
// varies so find it by class.
// This only exists with QsRevamp.
var FeatureTileNetwork = nodewith.HasClass("NetworkFeatureTile")

// FeatureTileScreenCapture is the finder for the "Screen capture" feature tile.
// This only exists with QsRevamp.
var FeatureTileScreenCapture = nodewith.HasClass("FeatureTile").NameContaining("Screen capture")

// LiveCaptionButton is the finder for the "Toggle Live Caption" button.
// It is a top-level button with QsRevamp.
var LiveCaptionButton = nodewith.Role(role.ToggleButton).NameContaining("Live Caption")

// NightLightButton is the finder for the "Toggle Night Light" button.
// It is a top-level button with QsRevamp.
var NightLightButton = nodewith.Role(role.ToggleButton).NameContaining("Night Light")

// DisplaySettingsButton is the finder for the "Show display settings" drill-in button.
// It is a top-level button with QsRevamp.
var DisplaySettingsButton = nodewith.Role(role.Button).NameContaining("display settings")

// FeatureTileDarkTheme is the finder for the "Toggle Dark theme" feature tile.
// It lives in the display settings detail page with QsRevamp.
var FeatureTileDarkTheme = nodewith.HasClass("FeatureTile").NameContaining("Dark theme")

// PowerMenuButton is the finder for the power menu button.
// It only exists with QsRevamp.
var PowerMenuButton = nodewith.Role(role.Button).NameContaining("Power menu")

// PowerMenuLockItem is the finder for the "Lock" item in the power menu.
// It only exists with QsRevamp.
var PowerMenuLockItem = nodewith.Role(role.MenuItem).NameContaining("Lock")

// PowerMenuSignOutItem is the finder for the "Sign out" item in the power menu.
// It only exists with QsRevamp.
var PowerMenuSignOutItem = nodewith.Role(role.MenuItem).NameContaining("Sign out")

// CollapseButton is the finder for the collapse button, which collapses Quick Settings.
// This button does not exist with QsRevamp.
var CollapseButton = nodewith.Role(role.Button).HasClass("CollapseButton").Name("Collapse menu")

// ExpandButton is the finder for the expand button, which expands Quick Settings.
// This button does not exist with QsRevamp.
var ExpandButton = nodewith.Role(role.Button).HasClass("CollapseButton").Name("Expand menu")

// LockButton is the finder for Quick Settings' lock button.
// This button does not exist with QsRevamp.
var LockButton = nodewith.Name("Lock").HasClass("IconButton")

// SettingsButton is the finder for the Quick Settings' setting button.
// This button is the same pre- and post-QsRevamp.
var SettingsButton = nodewith.Name("Settings").HasClass("IconButton")

// ShutdownButton is the finder for the shutdown button in Quick Settings.
// This button does not exist with QsRevamp.
var ShutdownButton = nodewith.Name("Shut down").HasClass("IconButton")

// SignoutButton is the finder for the 'Sign out' Quick Settings button.
var SignoutButton = nodewith.Role(role.Button).Name("Sign out").HasClass("PillButton")

// VPNButton is the finder for the 'VPN' Quick Settings button.
var VPNButton = nodewith.Role(role.Button).Name("VPN").HasClass("PillButton")

// SliderType represents the Quick Settings slider elements.
type SliderType string

// List of descriptive slider names. These don't correspond to any UI node attributes,
// but will be used as keys to map descriptive names to the finders defined below.
const (
	SliderTypeVolume     SliderType = "Volume"
	SliderTypeBrightness SliderType = "Brightness"
	SliderTypeMicGain    SliderType = "Mic gain"
)

// SliderParamMap maps slider names (SliderType) to the  to find the sliders in the UI.
var SliderParamMap = map[SliderType]*nodewith.Finder{
	SliderTypeVolume:     VolumeSlider,
	SliderTypeBrightness: BrightnessSlider,
	SliderTypeMicGain:    MicGainSlider,
}

// BrightnessSlider is the finder for the Quick Settings brightness slider.
var BrightnessSlider = nodewith.Name("Brightness").HasClass("QuickSettingsSlider").Role(role.Slider)

// VolumeSlider is the finder for the Quick Settings volume slider.
var VolumeSlider = nodewith.Name("Volume").HasClass("QuickSettingsSlider").Role(role.Slider)

// VolumeToggle is the finder for the button that toggles the volume's mute status.
var VolumeToggle = nodewith.Role(role.ToggleButton).NameStartingWith("Toggle Volume")

// MicGainSlider is the finder for the Quick Settings mic gain slider.
// The Finder is identical to the volume slider, but it's located on a different
// page of Quick Settings.
var MicGainSlider = nodewith.Name("Microphone").HasClass("QuickSettingsSlider").Role(role.Slider)

// MicToggle is the finder for the button that toggles the microphone's mute status.
var MicToggle = nodewith.Role(role.ToggleButton).Attribute("name", regexp.MustCompile("Toggle Mic"))

// ManagedInfoView is the finder for the Quick Settings management information display.
var ManagedInfoView = nodewith.Role(role.Button).HasClass("EnterpriseManagedView")

// BatteryView is the finder for the Quick Settings battery display.
var BatteryView = nodewith.Role(role.LabelText).NameContaining("Battery")

// DateView is the finder for the Quick Settings date/time display.
// This view does not exist with QsRevamp.
var DateView = nodewith.Role(role.Button).HasClass("DateView")

// SettingPod represents the name of a setting pod in Quick Settings.
// These names are contained in the Name attribute of the automation node
// for the corresponding pod icon button, so they can be used to find the
// buttons in the UI.
type SettingPod string

// List of quick setting names, derived from the corresponding pod icon button node names.
// Character case in the names should exactly match the pod icon button node Name attribute.
// These nodes do not exist with QsRevamp.
const (
	SettingPodAccessibility     SettingPod = "accessibility"
	SettingPodBluetooth         SettingPod = "Bluetooth"
	SettingPodDoNotDisturb      SettingPod = "Do not disturb"
	SettingPodNetwork           SettingPod = "network"
	SettingPodNightLight        SettingPod = "Night Light"
	SettingPodNearbyShare       SettingPod = "Nearby Share"
	SettingPodKeyboard          SettingPod = "keyboard"
	SettingPodScreenCapture     SettingPod = "Screen capture"
	SettingPodVPN               SettingPod = "VPN"
	SettingPodDarkTheme         SettingPod = "Toggle Dark theme"
	SettingPodDarkThemeSettings SettingPod = "dark theme settings"
	SettingPodCast              SettingPod = "cast"
	SettingPodCameraFraming     SettingPod = "Camera framing"
)
