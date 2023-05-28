// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vctray

import (
	"context"
	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DevType represents the AV device type.
type DevType string

// Available options of DevType.
const (
	DevMicrophone DevType = "Microphone"
	DevCamera             = "Camera"
	DevScreen             = "Screen"
)

// AVState represents the state of VC device state indicated in vcTray.
type AVState string

// Available options of AVState.
const (
	DeviceDisabled     AVState = "disabled"
	DeviceAvailable            = "available"
	DeviceInUse                = "in use"
	DeviceHidden               = "hidden"
	DeviceUnknownState         = "unknown" // Should only be used on error.
)

func devFinder(devName DevType) *nodewith.Finder {
	if devName == DevScreen {
		return nodewith.Role(role.ToggleButton).NameContaining(string(devName)).Ancestor(vcTraySection)
	}
	return nodewith.Role(role.ToggleButton).NameContaining(`Toggle ` + string(devName)).Ancestor(vcTraySection)
}

// DevState returns the current state of a certain av device.
func (vcTray VCTray) DevState(ctx context.Context, devName DevType) (AVState, error) {
	if isNodeFound, err := vcTray.ui.IsNodeFound(ctx, devFinder(devName)); err != nil {
		return DeviceUnknownState, errors.Wrapf(err, "failed to check %v", devFinder(devName))
	} else if !isNodeFound {
		return DeviceHidden, nil
	}

	nodeInfo, err := vcTray.ui.Info(ctx, devFinder(devName))
	if err != nil {
		return DeviceUnknownState, errors.Wrap(err, "failed to get node info")
	}
	if strings.Contains(nodeInfo.Name, "in use") {
		return DeviceInUse, nil
	} else if strings.Contains(nodeInfo.Name, "is on") {
		return DeviceAvailable, nil
	} else if strings.Contains(nodeInfo.Name, "is off") {
		return DeviceDisabled, nil
	}
	return DeviceUnknownState, errors.Errorf("unknown node name: %q", nodeInfo.Name)
}

// WaitUntilState until the device state to be expected.
func (vcTray VCTray) WaitUntilState(devName DevType, expectedState AVState) action.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			currentDevState, err := vcTray.DevState(ctx, devName)
			if err != nil {
				return err
			}
			if currentDevState != expectedState {
				return errors.Errorf("failed to verify %s status; expected %v, got %v", string(devName), expectedState, currentDevState)
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second})
	}
}

// ToggleAVDevice clicks the icon button in vcTray to change device status.
// It gets the current status first and skip action if it is already expected.
func (vcTray VCTray) ToggleAVDevice(devName DevType, expectedOn bool) action.Action {
	return func(ctx context.Context) error {
		devState, err := vcTray.DevState(ctx, devName)
		if err != nil {
			return err
		}
		if (devState == DeviceDisabled && !expectedOn) || devState != DeviceDisabled && expectedOn {
			return nil
		}

		return vcTray.ui.DoDefaultUntil(
			devFinder(devName),
			func(ctx context.Context) error {
				return testing.Poll(ctx, func(ctx context.Context) error {
					devState, err := vcTray.DevState(ctx, devName)
					if err != nil {
						return err
					}

					if (devState == DeviceDisabled && !expectedOn) || devState != DeviceDisabled && expectedOn {
						return nil
					}

					return errors.Errorf("failed to toggle %s to %v, latest state: %v", devName, expectedOn, devState)
				}, &testing.PollOptions{Timeout: 5 * time.Second})
			},
		)(ctx)
	}
}
