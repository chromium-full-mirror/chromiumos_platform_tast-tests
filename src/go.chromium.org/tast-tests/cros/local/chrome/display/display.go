// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package display wraps the chrome.system.display API.
//
// Functions require a chrome.Conn with permission to use the chrome.system.display API.
// chrome.Chrome.TestAPIConn has such permission and may be passed here.
package display

import (
	"context"
	"math"
	"os"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Insets holds onscreen insets.
// See https://developer.chrome.com/docs/extensions/reference/system_display/#type-Insets.
type Insets struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
}

// DisplayMode holds a mode supported by the display.
// See https://developer.chrome.com/docs/extensions/reference/system_display/#type-DisplayMode.
type DisplayMode struct { // NOLINT
	Width                int     `json:"width"`
	Height               int     `json:"height"`
	WidthInNativePixels  int     `json:"widthInNativePixels"`
	HeightInNativePixels int     `json:"heightInNativePixels"`
	UIScale              float64 `json:"uiScale,omitempty"`
	DeviceScaleFactor    float64 `json:"deviceScaleFactor"`
	RefreshRate          float64 `json:"refreshRate"`
	IsNative             bool    `json:"isNative"`
	IsSelected           bool    `json:"isSelected"`
	IsInterlaced         bool    `json:"isInterlaced,omitempty"`
}

// Info holds information about a display and is returned by GetInfo.
// See https://developer.chrome.com/docs/extensions/reference/system_display/#type-DisplayUnitInfo.
type Info struct {
	ID                          string         `json:"id"`
	Name                        string         `json:"name"`
	MirroringSourceID           string         `json:"mirroringSourceId"`
	IsPrimary                   bool           `json:"isPrimary"`
	IsInternal                  bool           `json:"isInternal"`
	IsEnabled                   bool           `json:"isEnabled"`
	IsUnified                   bool           `json:"isUnified"`
	DPIX                        float64        `json:"dpiX"`
	DPIY                        float64        `json:"dpiY"`
	Rotation                    int            `json:"rotation"`
	Bounds                      coords.Rect    `json:"bounds"`
	Overscan                    *Insets        `json:"overscan"`
	WorkArea                    coords.Rect    `json:"workArea"`
	Modes                       []*DisplayMode `json:"modes"`
	HasTouchSupport             bool           `json:"hasTouchSupport"`
	AvailableDisplayZoomFactors []float64      `json:"availableDisplayZoomFactors"`
	DisplayZoomFactor           float64        `json:"displayZoomFactor"`
}

// GetSelectedMode returns the currently selected display mode. It returns
// nil if no such mode is found.
func (info *Info) GetSelectedMode() (*DisplayMode, error) {
	for _, mode := range info.Modes {
		if mode.IsSelected {
			return mode, nil
		}
	}
	return nil, errors.New("no modes are selected")
}

// GetEffectiveDeviceScaleFactor computes the ratio of a DIP (device independent
// pixel) to a physical pixel, which is DisplayZoomFactor x DeviceScaleFactor.
// See also ui/display/manager/managed_display_info.h in Chromium.
func (info *Info) GetEffectiveDeviceScaleFactor() (float64, error) {
	mode, err := info.GetSelectedMode()
	if err != nil {
		return 0, err
	}

	scaleFactor := info.DisplayZoomFactor * mode.DeviceScaleFactor
	// Make sure the scale factor is neither 0 nor NaN.
	if math.IsNaN(scaleFactor) || math.Abs(scaleFactor) < 1e-10 {
		return 0, errors.Errorf("invalid device scale factor: %f", scaleFactor)
	}

	return scaleFactor, nil
}

// GetInfo calls chrome.system.display.getInfo to get information about connected displays.
// See https://developer.chrome.com/docs/extensions/reference/system_display/#method-getInfo.
func GetInfo(ctx context.Context, tconn *chrome.TestConn) ([]Info, error) {
	var infos []Info
	if err := tconn.Call(ctx, &infos, `tast.promisify(chrome.system.display.getInfo)`); err != nil {
		return nil, errors.Wrap(err, "failed to get display info")
	}
	if len(infos) == 0 {
		// At leasat one display info should exist. So empty info would mean
		// something is wrong.
		return nil, errors.New("no display info are contained")
	}
	return infos, nil
}

// FindInfo returns information about the display that satisfies the given predicate.
func FindInfo(ctx context.Context, tconn *chrome.TestConn, predicate func(info *Info) bool) (*Info, error) {
	infos, err := GetInfo(ctx, tconn)
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		if predicate(&info) {
			return &info, nil
		}
	}
	return nil, errors.New("failed to find a display satisfying the condition")
}

// GetInternalInfo returns information about the internal display.
// An error is returned if no internal display is present.
func GetInternalInfo(ctx context.Context, tconn *chrome.TestConn) (*Info, error) {
	return FindInfo(ctx, tconn, func(info *Info) bool {
		return info.IsInternal
	})
}

// GetPrimaryInfo returns information about the primary display.
func GetPrimaryInfo(ctx context.Context, tconn *chrome.TestConn) (*Info, error) {
	return FindInfo(ctx, tconn, func(info *Info) bool {
		return info.IsPrimary
	})
}

// GetInfoForID returns information about the display with a specific ID.
func GetInfoForID(ctx context.Context, tconn *chrome.TestConn, id string) (*Info, error) {
	return FindInfo(ctx, tconn, func(info *Info) bool {
		return info.ID == id
	})
}

// GetDeviceScaleFactor returns information about the Device scale factor
// of the first display that satisfy the function match.
// If there no matching function, an error will be returned.
func GetDeviceScaleFactor(ctx context.Context, tconn *chrome.TestConn, match func(info *Info) bool) (float64, error) {

	displayInfo, err := FindInfo(ctx, tconn, match)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get the display info")
	}
	displayMode, err := displayInfo.GetSelectedMode()
	if err != nil {
		return 0, errors.Wrap(err, "failed to get the selected display mode of the display")
	}
	return displayMode.DeviceScaleFactor, nil
}

// DisplayProperties holds properties to change and is passed to SetDisplayProperties.
// nil fields are ignored. See https://developer.chrome.com/docs/extensions/reference/system_display/#type-DisplayProperties.
type DisplayProperties struct { // NOLINT
	IsUnified         *bool        `json:"isUnified,omitempty"`
	MirroringSourceID *string      `json:"mirroringSourceId,omitempty"`
	IsPrimary         *bool        `json:"isPrimary,omitempty"`
	Overscan          *Insets      `json:"overscan,omitempty"`
	Rotation          *int         `json:"rotation,omitempty"`
	BoundsOriginX     *int         `json:"boundsOriginX,omitempty"`
	BoundsOriginY     *int         `json:"boundsOriginY,omitempty"`
	DisplayMode       *DisplayMode `json:"displayMode,omitempty"`
	DisplayZoomFactor *float64     `json:"displayZoomFactor,omitempty"`
}

// SetDisplayProperties updates the properties for the display specified by id.
// See https://developer.chrome.com/docs/extensions/reference/system_display/#method-setDisplayProperties.
// Some properties, like rotation, will be performed in an async way. For rotation in particular,
// you should call display.WaitForDisplayRotation() to know when the rotation animation finishes.
func SetDisplayProperties(ctx context.Context, tconn *chrome.TestConn, id string, dp DisplayProperties) error {
	return tconn.Call(ctx, nil, "tast.promisify(chrome.system.display.setDisplayProperties)", id, dp)
}

// SetDisplayMode sets the display to a specific mode. The mode must be a
// supported one from GetModes(). Only works on external displays or VM with
// vkms enabled.
func SetDisplayMode(ctx context.Context, tconn *chrome.TestConn, id string, mode *DisplayMode) error {
	ui := uiauto.New(tconn)

	if err := SetDisplayProperties(ctx, tconn, id, DisplayProperties{DisplayMode: mode}); err != nil {
		return errors.Wrap(err, "failed to set display properties")
	}

	// Dismiss confirm dialog
	confirmButton := nodewith.Name("Confirm").Role(role.Button)
	if err := uiauto.Combine("Click confirm button",
		ui.DoDefault(confirmButton),
		ui.WaitUntilGone(confirmButton))(ctx); err != nil {
		return errors.Wrap(err, "failed to click 'Confirm' button")
	}

	newDispInfo, err := GetInfoForID(ctx, tconn, id)
	if err != nil {
		return errors.Wrap(err, "failed to get updated display info")
	}

	newMode, err := newDispInfo.GetSelectedMode()
	if err != nil {
		return errors.Wrap(err, "failed to get updated mode")
	}

	if diff := cmp.Diff(mode, newMode, cmpopts.IgnoreFields(DisplayMode{}, "IsSelected")); diff != "" {
		return errors.Wrapf(err, "new mode does not match requested (-want +got): %s", diff)
	}

	return nil
}

// RotationAngle represents the supported rotation angles by SetDisplayRotationSync.
type RotationAngle string

// Rotation values as defined in: https://cs.chromium.org/chromium/src/out/Debug/gen/chrome/common/extensions/api/autotest_private.h
const (
	// RotateAny represents the auto-rotation to the device angle. Valid only in
	// tablet mode.
	RotateAny RotationAngle = "RotateAny"
	// Rotate0 represents rotation angle 0.
	Rotate0 RotationAngle = "Rotate0"
	// Rotate90 represents rotation angle 90.
	Rotate90 RotationAngle = "Rotate90"
	// Rotate180 represents rotation angle 180.
	Rotate180 RotationAngle = "Rotate180"
	// Rotate270 represents rotation angle 270.
	Rotate270 RotationAngle = "Rotate270"
)

// WaitForDisplayRotation waits for the display rotation animation. If it is not
// animating, it returns immediately. Returns an error if incorrect parameters
// are passed, or the display rotation ends up with a different rotation from
// the specified one.
func WaitForDisplayRotation(ctx context.Context, tconn *chrome.TestConn, dispID string, rot RotationAngle) error {
	return tconn.Call(ctx, nil, `async (displayId, rotation) => {
	  if (!await tast.promisify(chrome.autotestPrivate.waitForDisplayRotation)(displayId, rotation))
	    throw new Error("failed to wait for display rotation");
	}`, dispID, rot)
}

// SetDisplayRotationSync rotates the display to a certain angle and waits until the rotation animation finished.
// c must be a connection with both system.display and autotestPrivate permissions.
func SetDisplayRotationSync(ctx context.Context, tconn *chrome.TestConn, dispID string, rot RotationAngle) error {
	var rotInt int
	switch rot {
	case RotateAny:
		rotInt = -1
	case Rotate0:
		rotInt = 0
	case Rotate90:
		rotInt = 90
	case Rotate180:
		rotInt = 180
	case Rotate270:
		rotInt = 270
	default:
		return errors.Errorf("unexpected rotation value; got %q, want: any of [Rotate0,Rotate90,Rotate180,Rotate270]", rot)
	}

	p := DisplayProperties{Rotation: &rotInt}
	if err := SetDisplayProperties(ctx, tconn, dispID, p); err != nil {
		return errors.Wrapf(err, "failed to set rotation to %d", rotInt)
	}

	return WaitForDisplayRotation(ctx, tconn, dispID, rot)
}

// OrientationType represents a display orientation.
type OrientationType string

// OrientationType values as "enum OrientationType" defined in https://w3c.github.io/screen-orientation/#screenorientation-interface
const (
	OrientationPortraitPrimary    OrientationType = "portrait-primary"
	OrientationPortraitSecondary  OrientationType = "portrait-secondary"
	OrientationLandscapePrimary   OrientationType = "landscape-primary"
	OrientationLandscapeSecondary OrientationType = "landscape-secondary"
)

// Orientation holds information obtained from the screen orientation API.
// See https://w3c.github.io/screen-orientation/#screenorientation-interface
type Orientation struct {
	// Angle is an angle in degrees of the display counterclockwise from the
	// orientation of the display panel.
	Angle int `json:"angle"`
	// Type is an OrientationType representing the display orientation.
	Type OrientationType `json:"type"`
}

// GetOrientation returns the Orientation of the display.
func GetOrientation(ctx context.Context, tconn *chrome.TestConn) (*Orientation, error) {
	result := &Orientation{}
	// Using a JS expression to evaluate screen.orientation to a JSON object
	// because JSON.stringify does not work for it and returns {}.
	if err := tconn.Eval(ctx, `s=screen.orientation;o={"angle":s.angle,"type":s.type}`, result); err != nil {
		return nil, err
	}
	return result, nil
}

// IsFakeDisplayID checks if a display is fake or not by its id.
func IsFakeDisplayID(id string) bool {
	// the id of fake displays will start from this number.
	// See also: https://source.chromium.org/chromium/chromium/src/+/HEAD:ui/display/manager/managed_display_info.cc?q=%20kSynthesizedDisplayIdStart
	const fakeDisplayID = "2200000000"

	// Theoretically it is possible that a fake display has a different ID. This
	// happens when some displays are connected and then disconnected; this is
	// unlikely to happen on test environment.
	return id == fakeDisplayID
}

// PhysicalDisplayConnected checks the display info and returns true if at least
// one physical display is connected.
func PhysicalDisplayConnected(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	infos, err := GetInfo(ctx, tconn)
	if err != nil {
		return false, err
	}
	if len(infos) > 1 {
		return true, nil
	}
	return !IsFakeDisplayID(infos[0].ID), nil
}

var intToRotationAngle = map[int]RotationAngle{
	0:   Rotate0,
	90:  Rotate90,
	180: Rotate180,
	270: Rotate270,
	-1:  RotateAny,
}

// RotateToLandscape rotates the display only if current orientation type is "portrait", and returns
// a function that restores the original orientation setting.
func RotateToLandscape(ctx context.Context, tconn *chrome.TestConn) (func(context.Context) error, error) {
	orientation, err := GetOrientation(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the orientation info")
	}
	testing.ContextLogf(ctx, "Current orientation type: %d; orientation angle: %v", orientation.Angle, orientation.Type)

	// Rotate the display only if current orientation is in portrait position.
	if orientation.Type == OrientationPortraitPrimary || orientation.Type == OrientationPortraitSecondary {
		info, err := GetPrimaryInfo(ctx, tconn)
		if err != nil {
			return nil, errors.Wrap(err, "failed to obtain primary display info")
		}
		restoreRotation := intToRotationAngle[info.Rotation]
		testing.ContextLog(ctx, "Current rotation setting: ", restoreRotation)

		// If the orientation angle is equal to 0 or 180, rotate 270 (or 90) degrees.
		// If the orientation angle is equal to 90 or 270, rotate 0 (or 180) degrees.
		var targetRotation RotationAngle
		if orientation.Angle == 0 || orientation.Angle == 180 {
			targetRotation = Rotate270
		} else {
			targetRotation = Rotate0
		}

		testing.ContextLog(ctx, "Target rotation setting: ", targetRotation)
		if err := SetDisplayRotationSync(ctx, tconn, info.ID, targetRotation); err != nil {
			return nil, err
		}
		return func(ctx context.Context) error {
			return SetDisplayRotationSync(ctx, tconn, info.ID, restoreRotation)
		}, nil
	}

	return func(context.Context) error {
		return nil
	}, nil
}

// RotationToAngle converts Info.Rotation value to RotationAngle.
func RotationToAngle(rot int) (RotationAngle, error) {
	if val, ok := intToRotationAngle[rot]; ok {
		return val, nil
	}
	return RotateAny, errors.Errorf("invalid rotation angle %d", rot)
}

// CheckExtendedDisplay makes sure there are two displays on DUT and the extended display has the expected mode.
// This procedure must be performed after display mirror is unset. Otherwise it can only get one display info.
func CheckExtendedDisplay(tconn *chrome.TestConn, expectedMode DisplayMode) action.Action {
	return func(ctx context.Context) error {
		infos, err := GetInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get display info")
		}
		if len(infos) != 2 {
			return errors.Wrapf(err, "DUT connected with incorrect number of displays - want 2, got %d", len(infos))
		}
		for _, info := range infos {
			if info.IsInternal {
				continue
			}
			mode, err := info.GetSelectedMode()
			if err != nil {
				return errors.Wrap(err, "failed to get selected mode")
			}
			if mode.Height != expectedMode.Height {
				return errors.Errorf("the height of the extended display is not as expected: got: %v, want: %v", mode.Height, expectedMode.Height)
			}
			if mode.RefreshRate != expectedMode.RefreshRate {
				return errors.Errorf("the refresh rate of the extended display is not as expected: got: %v, want: %v", mode.RefreshRate, expectedMode.RefreshRate)
			}
		}
		return nil
	}
}

// MinimizePrimaryDisplayZoomFactor sets the zoom factor of the primary display to
// the smallest available. If successful, this function returns a function that
// reverts the zoom factor. MinimizePrimaryDisplayZoomFactor is useful for
// ensuring that the minimum size of a browser window is conducive to split view.
func MinimizePrimaryDisplayZoomFactor(ctx context.Context, tconn *chrome.TestConn) (
	func(context.Context, *chrome.TestConn) error,
	error,
) {
	info, err := GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the primary display info")
	}

	zoomInitial := info.DisplayZoomFactor

	zoomMin := math.Inf(1)
	for _, zoom := range info.AvailableDisplayZoomFactors {
		if zoom < zoomMin {
			zoomMin = zoom
		}
	}

	if err := SetDisplayProperties(ctx, tconn, info.ID, DisplayProperties{DisplayZoomFactor: &zoomMin}); err != nil {
		return nil, errors.Wrapf(err, "failed to set zoom factor of primary display to minimum %f", zoomMin)
	}
	return func(ctx context.Context, tconn *chrome.TestConn) error {
		if err := SetDisplayProperties(ctx, tconn, info.ID, DisplayProperties{DisplayZoomFactor: &zoomInitial}); err != nil {
			return errors.Wrapf(err, "failed to revert zoom factor of primary display to %f", zoomInitial)
		}
		return nil
	}, nil
}

// PSRState is an enumeration to control PSR behavior in tests.
type PSRState string

// PSRState values correspond to valid values for i915_edp_psr_status.
const (
	PSRDefault      PSRState = "PSRDefault"
	PSRForceDisable PSRState = "PSRForceDisable"
	PSRForceEnable  PSRState = "PSRForceEnable"
)

// SetPSRState sets PSR state. Only supports i915 platforms.
func SetPSRState(state PSRState) error {
	// This file exists on all i915 devices regardless of psr support, so we cannot
	// rely on the file's existence to infer psr support. Reading from this file
	// on an i915 device without PSR support will fail with ENODEV.
	edpPsrStatusPath := "/sys/kernel/debug/dri/0/i915_edp_psr_debug"
	if _, err := os.ReadFile(edpPsrStatusPath); err != nil {
		// It's not a failure to set to default if PSR is not supported.
		if state == PSRDefault {
			return nil
		}
		return errors.New("SetPSRState is only supported on i915 devices with PSR")
	}

	// stateString is the value to be written to the psr debug file.
	var stateString []byte
	switch state {
	case PSRDefault:
		stateString = []byte("0")
	case PSRForceDisable:
		stateString = []byte("1")
	case PSRForceEnable:
		stateString = []byte("2")
	}

	if err := os.WriteFile(edpPsrStatusPath, stateString, 0644); err != nil {
		return errors.Wrapf(err, "could not write to file %s", edpPsrStatusPath)
	}
	return nil
}
