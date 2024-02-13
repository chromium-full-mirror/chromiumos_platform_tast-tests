// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"regexp"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// QsRootFinder is the finder to find the Quick Settings area in the UI.
var QsRootFinder = nodewith.HasClass("QuickSettingsView")

// NBSWarningLabel is the finder to find the NBS Warning Label in the UI.
// This is only available when an NBS BT audio device is connected and chosen.
var NBSWarningLabel = nodewith.Role(role.StaticText).NameStartingWith("Your Chromebook or Bluetooth device is using an older version of Bluetooth.")

// SystemTray is to distinguish it from calendar view in cases that use the calendar, it is also the finder to find the Quick Settings area in the UI.
var SystemTray = nodewith.HasClass("UnifiedSystemTray")

// StatusAreaWidget is the finder to find the control widgets.
var StatusAreaWidget = nodewith.Role(role.Pane).ClassNameRegex(regexp.MustCompile(ash.StatusAreaClassNameRegex))

// FeatureTileAccessibility is the finder for the "Accessibility" feature tile.
var FeatureTileAccessibility = nodewith.HasClass("FeatureTile").NameContaining("accessibility")

// FeatureTileBluetooth is the finder for the "Bluetooth" feature tile.
var FeatureTileBluetooth = nodewith.HasClass("FeatureTile").NameContaining("Bluetooth")

// FeatureTileBluetoothToggle is the finder for the toggle button that is part
// of the Bluetooth feature tile.
var FeatureTileBluetoothToggle = nodewith.Role(role.Button).NameContaining("Toggle Bluetooth").Ancestor(FeatureTileBluetooth)

// FeatureTileCameraFraming is the finder for the "Camera framing" feature tile.
var FeatureTileCameraFraming = nodewith.Role(role.ToggleButton).NameContaining("Camera framing")

// FeatureTileCast is the finder for the "Cast screen" feature tile.
var FeatureTileCast = nodewith.HasClass("FeatureTile").NameContaining("cast")

// FeatureTileHotspot is the finder for the "Hotspot" feature tile.
var FeatureTileHotspot = nodewith.HasClass("FeatureTile").NameContaining("Hotspot")

// FeatureTileDoNotDisturb is the finder for the "Do not disturb" feature tile.
var FeatureTileDoNotDisturb = nodewith.Role(role.ToggleButton).HasClass("FeatureTile").NameRegex(regexp.MustCompile("(?i)Do not disturb"))

// FeatureTileKeyboard is the finder for the "Keyboard" (IME) feature tile.
var FeatureTileKeyboard = nodewith.HasClass("FeatureTile").NameContaining("keyboard")

// FeatureTileNearbyShare is the finder for the "Nearby Share" feature tile.
// TODO(b/314352172): Change string after flag is enabled.
var FeatureTileNearbyShare = nodewith.HasClass("FeatureTile").NameContaining("Nearby Share")

// FeatureTileNetwork is the finder for the network feature tile. Its name
// varies so find it by class.
var FeatureTileNetwork = nodewith.HasClass("NetworkFeatureTile")

// FeatureTileScreenCapture is the finder for the "Screen capture" feature tile.
var FeatureTileScreenCapture = nodewith.HasClass("FeatureTile").NameContaining("Screen capture")

// LiveCaptionButton is the finder for the "Toggle Live Caption" button.
var LiveCaptionButton = nodewith.Role(role.ToggleButton).NameContaining("Live Caption")

// NightLightButton is the finder for the "Toggle Night Light" button.
var NightLightButton = nodewith.Role(role.ToggleButton).NameContaining("Night Light")

// DisplaySettingsButton is the finder for the "Show display settings" drill-in button.
var DisplaySettingsButton = nodewith.Role(role.Button).NameContaining("display settings")

// FeatureTileDarkTheme is the finder for the "Toggle Dark theme" feature tile.
// It lives in the display settings detail page.
var FeatureTileDarkTheme = nodewith.HasClass("FeatureTile").NameContaining("Dark theme")

// PowerMenuButton is the finder for the power menu button.
var PowerMenuButton = nodewith.Role(role.Button).NameContaining("Power menu")

// PowerMenuLockItem is the finder for the "Lock" item in the power menu.
var PowerMenuLockItem = nodewith.Role(role.MenuItem).NameContaining("Lock")

// PowerMenuSignOutItem is the finder for the "Sign out" item in the power menu.
var PowerMenuSignOutItem = nodewith.Role(role.MenuItem).NameContaining("Sign out")

// SettingsButton is the finder for the Quick Settings' setting button.
var SettingsButton = nodewith.Name("Settings").HasClass("IconButton")

// SignoutButton is the finder for the 'Sign out' Quick Settings button.
var SignoutButton = nodewith.Role(role.Button).Name("Sign out").HasClass("PillButton")

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
var BatteryView = nodewith.Role(role.StaticText).NameContaining("Battery")

// AddCellularButton is the finder for adding new SIM profiles in Quick Settings.
var AddCellularButton = nodewith.Name("Add eSIM").Role(role.Button)

// APNSubpageButton is the finder for the button on the details view of a cellular network that navigates to the APN details view for the network.
var APNSubpageButton = nodewith.NameContaining("Access point name").Role(role.Button).First()

// HotspotDetailedViewToggle is the finder for the hotspot toggle in Hotspot detailed view in Quick Settings.
var HotspotDetailedViewToggle = nodewith.Name("Toggle hotspot").Role(role.Switch)

// HotspotPolicyImage is the finder for the policy icon in Hotspot detailed view in Quick Settings.
var HotspotPolicyImage = nodewith.Name("This setting is managed by your administrator").Role(role.Image)

// HotspotOnNoDeviceConnectedText is the finder for the text that showing hotspot is on but no devices connected in Hotspot detailed view.
var HotspotOnNoDeviceConnectedText = nodewith.Name("No devices connected").Role(role.StaticText)

// HotspotTurningOnText is the finder for the text that showing hotspot is turning on in Hotspot detailed view.
var HotspotTurningOnText = nodewith.NameStartingWith("Turning on").Role(role.StaticText)
