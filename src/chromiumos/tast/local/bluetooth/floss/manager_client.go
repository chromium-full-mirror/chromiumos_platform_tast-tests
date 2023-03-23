// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/dbusutil"
	"chromiumos/tast/testing"
)

// ManagerClient handles method calls to the floss admin interface.
type ManagerClient struct {
	dbus *dbusutil.DBusObject
}

// NewManagerClient creates a new floss ManagerClient from the passed D-Bus
// object path.
func NewManagerClient(ctx context.Context, path dbus.ObjectPath) (*ManagerClient, error) {
	obj, err := NewFlossManagerDBusObject(ctx, flossManagerInterface, path)
	if err != nil {
		return nil, err
	}
	return &ManagerClient{dbus: obj}, nil
}

// ManagerClients creates a ManagerClient for all floss managers in the system.
func ManagerClients(ctx context.Context) ([]*ManagerClient, error) {
	paths, err := collectExistingFlossManagerServiceObjectPaths(ctx, flossManagerInterface)
	if err != nil {
		return nil, err
	}
	managerClients := make([]*ManagerClient, len(paths))
	for i, path := range paths {
		managerClient, err := NewManagerClient(ctx, path)
		if err != nil {
			return nil, err
		}
		managerClients[i] = managerClient
	}
	return managerClients, nil
}

// DefaultManagerClient returns an initialized ManagerClient for the default
// Manager object.
func DefaultManagerClient(ctx context.Context) (*ManagerClient, error) {
	managers, err := ManagerClients(ctx)
	if err != nil {
		return nil, err
	}
	if len(managers) != 1 {
		return nil, errors.Errorf("expected exactly 1 floss manager to exist, but found %d", len(managers))
	}
	return managers[0], nil
}

// DBusObject returns the D-Bus object wrapper for this object.
func (c *ManagerClient) DBusObject() *dbusutil.DBusObject {
	return c.dbus
}

// DefaultAdapterHCI returns the default floss adapter HCI.
func (c *ManagerClient) DefaultAdapterHCI() int {
	return defaultFlossAdapterHCI
}

// Start starts the given adapter.
func (c *ManagerClient) Start(ctx context.Context, adapterHCI int) error {
	call := c.dbus.Call(ctx, "Start", adapterHCI)
	if call.Err != nil {
		return errors.Wrapf(call.Err, "failed to start adapter hci%d", adapterHCI)
	}
	return nil
}

// Stop stops the given adapter.
func (c *ManagerClient) Stop(ctx context.Context, adapterHCI int) error {
	call := c.dbus.Call(ctx, "Stop", adapterHCI)
	if call.Err != nil {
		return errors.Wrapf(call.Err, "failed to stop adapter hci%d", adapterHCI)
	}
	return nil
}

// GetAdapterEnabled returns the enabled state of the given adapter hci.
func (c *ManagerClient) GetAdapterEnabled(ctx context.Context, adapterHCI int) (bool, error) {
	return c.dbus.CallForBool(ctx, "GetAdapterEnabled", adapterHCI)
}

// GetFlossEnabled returns the enabled state of floss.
func (c *ManagerClient) GetFlossEnabled(ctx context.Context) (bool, error) {
	return c.dbus.CallForBool(ctx, "GetFlossEnabled")
}

// SetFlossEnabled sets the enabled state of floss.
func (c *ManagerClient) SetFlossEnabled(ctx context.Context, enabled bool) error {
	call := c.dbus.Call(ctx, "SetFlossEnabled", enabled)
	if call.Err != nil {
		return errors.Wrap(call.Err, "failed to call SetFlossEnabled")
	}
	return nil
}

// GetAvailableAdapters calls the floss D-Bus method with the same name for this
// object.
func (c *ManagerClient) GetAvailableAdapters(ctx context.Context) ([]map[string]dbus.Variant, error) {
	call := c.dbus.Call(ctx, "GetAvailableAdapters")
	if call.Err != nil {
		return nil, errors.Wrap(call.Err, "failed to get available adapters")
	}
	var adapters []map[string]dbus.Variant
	if err := call.Store(&adapters); err != nil {
		return nil, errors.Wrap(err, "failed to store GetAvailableAdapters response")
	}
	return adapters, nil
}

// Enable starts the default adapter.
func Enable(ctx context.Context) error {
	manager, err := DefaultManagerClient(ctx)
	if err != nil {
		return err
	}
	return manager.Start(ctx, defaultFlossAdapterHCI)
}

// IsEnabled returns the power state of the default adapter.
func IsEnabled(ctx context.Context) (bool, error) {
	manager, err := DefaultManagerClient(ctx)
	if err != nil {
		return false, err
	}
	return manager.GetAdapterEnabled(ctx, defaultFlossAdapterHCI)
}

// PollForAdapterState polls the bluetooth adapter state until expected state is received or a timeout occurs.
func PollForAdapterState(ctx context.Context, exp bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		status, err := IsEnabled(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to check Bluetooth status"))
		}
		if status != exp {
			expStr := "disabled"
			curStr := "enabled"
			if exp {
				expStr, curStr = curStr, expStr
			}
			return errors.Errorf("failed to verify Bluetooth status, got %s, want %s", curStr, expStr)
		}

		return nil

	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second})
}

// PollForEnabled polls the bluetooth adapter state until the adapter is enabled.
func PollForEnabled(ctx context.Context) error {
	return PollForAdapterState(ctx, true)
}

// PollForAdapterAvailable polls at least one bluetooth adapter is available.
func PollForAdapterAvailable(ctx context.Context) error {
	manager, err := DefaultManagerClient(ctx)
	if err != nil {
		return err
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		adapters, err := manager.GetAvailableAdapters(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get available adapters"))
		}
		if len(adapters) == 0 {
			return errors.New("no available adapter found")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second})
}
