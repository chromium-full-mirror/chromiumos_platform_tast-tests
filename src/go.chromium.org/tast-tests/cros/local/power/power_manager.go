// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	pmpb "chromiumos/system_api/power_manager_proto"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dbusName      = "org.chromium.PowerManager"
	dbusPath      = "/org/chromium/PowerManager"
	dbusInterface = "org.chromium.PowerManager"
)

// PowerManager is used to interact with the powerd process over D-Bus.
// For detailed spec of each D-Bus method, please find
// src/platform2/power_manager/dbus_bindings/org.chromium.PowerManager.xml
type PowerManager struct { // NOLINT
	conn *dbus.Conn
	obj  dbus.BusObject
}

// UserActivityType is a status code for the PowerManager related D-Bus methods.
type UserActivityType int32

// NewPowerManager connects to power_manager via D-Bus and returns a PowerManager object.
func NewPowerManager(ctx context.Context) (*PowerManager, error) {
	conn, obj, err := dbusutil.Connect(ctx, dbusName, dbusPath)
	if err != nil {
		return nil, err
	}
	return &PowerManager{conn, obj}, nil
}

// GetSwitchStates calls PowerManager.GetSwitchStates D-Bus method.
func (m *PowerManager) GetSwitchStates(ctx context.Context) (*pmpb.SwitchStates, error) {
	ret := &pmpb.SwitchStates{}
	err := dbusutil.CallProtoMethod(ctx, m.obj, dbusInterface+".GetSwitchStates", nil, ret)
	return ret, err
}

// HandleWakeNotification calls PowerManager.HandleWakeNotification D-Bus method.
func (m *PowerManager) HandleWakeNotification(ctx context.Context) error {
	return dbusutil.CallProtoMethod(ctx, m.obj, dbusInterface+".HandleWakeNotification", nil, nil)
}

// GetScreenBrightnessPercent returns current screen brightness by calling PowerManager.GetScreenBrightnessPercent D-Bus method.
func (m *PowerManager) GetScreenBrightnessPercent(ctx context.Context) (float64, error) {
	call := m.obj.CallWithContext(ctx, dbusInterface+".GetScreenBrightnessPercent", 0)
	if call.Err != nil {
		return 0.0, errors.Wrap(call.Err, "failed to call GetScreenBrightnessPercent D-Bus method")
	}

	var brightness float64
	if err := call.Store(&brightness); err != nil {
		return 0.0, errors.Wrap(err, "failed to store GetScreenBrightnessPercent D-Bus method call response into float64 pointer")
	}
	return brightness, nil
}

// GetPowerSupplyProperties returns power supply information by calling PowerManager.GetPowerSupplyProperties D-Bus method.
func (m *PowerManager) GetPowerSupplyProperties(ctx context.Context) (*pmpb.PowerSupplyProperties, error) {
	ret := &pmpb.PowerSupplyProperties{}
	err := dbusutil.CallProtoMethod(ctx, m.obj, dbusInterface+".GetPowerSupplyProperties", nil, ret)
	return ret, err
}

// SetScreenBrightness updates the screen brightness to the specified percentage by calling
// PowerManager.SetScreenBrightness D-Bus method.
func (m *PowerManager) SetScreenBrightness(ctx context.Context, percentage float64) error {
	if err := dbusutil.CallProtoMethod(ctx, m.obj, dbusInterface+".SetScreenBrightness",
		&pmpb.SetBacklightBrightnessRequest{
			Percent: &percentage,
		}, nil); err != nil {
		return errors.Wrap(err, "failed to call SetScreenBrightness D-Bus method")
	}
	return nil
}

// SetPolicy sets the policy by calling PowerManager.SetPolicy D-Bus method.
func (m *PowerManager) SetPolicy(ctx context.Context, policy *pmpb.PowerManagementPolicy) error {
	if err := dbusutil.CallProtoMethod(ctx, m.obj, dbusInterface+".SetPolicy", policy, nil); err != nil {
		return errors.Wrap(err, "failed to call SetPolicy D-Bus method")
	}
	return nil
}

// SetBatterySaverModeState sets the battery saver mode state.
func (m *PowerManager) SetBatterySaverModeState(ctx context.Context, enabled bool) error {
	if err := dbusutil.CallProtoMethod(ctx, m.obj, dbusInterface+".SetBatterySaverModeState", &pmpb.SetBatterySaverModeStateRequest{Enabled: &enabled}, nil); err != nil {
		return errors.Wrap(err, "failed to call SetBatterySaverModeState D-Bus method")
	}
	return nil
}

// GetBatterySaverModeState gets the battery saver mode state.
func (m *PowerManager) GetBatterySaverModeState(ctx context.Context) (*pmpb.BatterySaverModeState, error) {
	ret := &pmpb.BatterySaverModeState{}
	err := dbusutil.CallProtoMethod(ctx, m.obj, dbusInterface+".GetBatterySaverModeState", nil, ret)
	return ret, err
}

// TurnOnDisplay turns on a display by sending a HandleWakeNotification to PowerManager
// to light up the display.
func TurnOnDisplay(ctx context.Context) error {
	// Emitting wake notification to powerd should finish quickly -- so setting
	// 10 seconds of timeout which should be long enough.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.EnsureJobRunning(ctx, "powerd"); err != nil {
		return errors.Wrap(err, "failed to ensure powerd running")
	}

	powerd, err := NewPowerManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a PowerManager object")
	}
	if err := powerd.HandleWakeNotification(ctx); err != nil {
		return errors.Wrap(err, "failed to call HandleWakeNotification D-Bus method")
	}
	return nil
}

// EnableBatterySaver checks that PowerManager is running, enables battery
// saver, and waits for the signal to propagate to components.
func EnableBatterySaver(ctx context.Context) error {
	// Enabling battery saver should finish quickly.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.EnsureJobRunning(ctx, "powerd"); err != nil {
		return errors.Wrap(err, "failed to ensure powerd running")
	}

	testing.ContextLog(ctx, "Enabling battery saver")

	powerd, err := NewPowerManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a PowerManager object")
	}

	if err := powerd.SetBatterySaverModeState(ctx, true); err != nil {
		return errors.Wrap(err, "failed to set battery saver state")
	}

	bsmState, err := powerd.GetBatterySaverModeState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get battery saver state")
	}
	if bsmState.Enabled == nil || !*(bsmState.Enabled) {
		return errors.New("battery saver is not enabled")
	}

	// GoBigSleepLint: Wait a bit to make sure the signal propagates everywhere.
	// There is no direct way to know if all battery saver levers have received
	// the signal, so we need to just sleep.
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for battery saver state to propegate")
	}
	return nil
}

// DisableBatterySaver waits for PowerManager to be running, verifies that
// battery saver is enabled, and then disables battery saver.
func DisableBatterySaver(ctx context.Context) error {
	// Turning off battery saver should finish quickly.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.EnsureJobRunning(ctx, "powerd"); err != nil {
		return errors.Wrap(err, "failed to ensure powerd running")
	}

	testing.ContextLog(ctx, "Disabling battery saver")

	powerd, err := NewPowerManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a PowerManager object")
	}

	// If powerd was disabled during the test, it might take a while for it to
	// come back.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		bsmState, err := powerd.GetBatterySaverModeState(ctx)
		if err != nil {
			return err
		}
		if bsmState == nil {
			return errors.New("bsmState is nil")
		}
		if bsmState.Enabled == nil || *bsmState.Enabled == false {
			return errors.New("bsmState.Enabled is not true")
		}
		return nil
	}, nil); err != nil {
		testing.ContextLog(ctx, "Failed to wait for powerd before disabling battery saver mode: ", err)
	}

	if err := powerd.SetBatterySaverModeState(ctx, false); err != nil {
		testing.ContextLog(ctx, "Failed to disable battery saver mode: ", err)
	}
	return nil
}
