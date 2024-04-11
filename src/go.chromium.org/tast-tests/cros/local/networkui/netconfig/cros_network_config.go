// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package netconfig contains the mojo connection to cros_network_config.
package netconfig

import (
	"context"
	"time"

	types "go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ErrNetworkNotFound indicates that the client was asked to do an operation on a network that could not be found.
// Most of the time, this will be returned if the network does not exist.
var ErrNetworkNotFound = errors.New("network not found")

// CrosNetworkConfig contains the mojo connection to cros_network_config.
type CrosNetworkConfig struct {
	conn       *chrome.Conn
	mojoRemote *chrome.JSObject
	isLoggedIn bool
}

// CreateOobeCrosNetworkConfig creates a connection to cros_network_config
// when the device is in the OOBE screen.
func CreateOobeCrosNetworkConfig(ctx context.Context, cr *chrome.Chrome) (*CrosNetworkConfig, error) {
	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open connection with oobe")
	}

	return NewCrosNetworkConfig(ctx, oobeConn, false /* isLoggedIn */)
}

// CreateLoggedInCrosNetworkConfig creates a connection to cros_network_config
// when the device is logged in so chrome://network may be opened.
func CreateLoggedInCrosNetworkConfig(ctx context.Context, cr *chrome.Chrome) (*CrosNetworkConfig, error) {
	conn, err := cr.NewConn(ctx, "chrome://network")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open network tab")
	}

	return NewCrosNetworkConfig(ctx, conn, true /* isLoggedIn */)
}

// NewCrosNetworkConfig creates a connection to cros_network_config that allows
// to make mojo calls. It receives a connection where it is possible to create
// a mojo connection to cros_network_config.
func NewCrosNetworkConfig(ctx context.Context, conn *chrome.Conn, isLoggedIn bool) (*CrosNetworkConfig, error) {
	var mojoRemote chrome.JSObject
	if err := conn.Call(ctx, &mojoRemote, crosNetworkConfigJs); err != nil {
		return nil, errors.Wrap(err, "failed to set up the network mojo API")
	}

	return &CrosNetworkConfig{
		conn:       conn,
		mojoRemote: &mojoRemote,
		isLoggedIn: isLoggedIn,
	}, nil
}

// Close cleans up the injected javascript.
func (c *CrosNetworkConfig) Close(ctx context.Context) error {
	if err := c.mojoRemote.Release(ctx); err != nil {
		return err
	}
	if c.isLoggedIn {
		if err := c.conn.CloseTarget(ctx); err != nil {
			return err
		}
	}
	return c.conn.Close()
}

// GetManagedProperties returns the managed properties of the given network,
// managed properties contain information on which values are set by policy or
// user. Look at cros_network_config.mojom or onc_spec.md for more information.
func (c *CrosNetworkConfig) GetManagedProperties(ctx context.Context, guid string) (*types.ManagedProperties, error) {
	var result *types.ManagedProperties
	if err := c.mojoRemote.Call(ctx, &result, "function(guid) {return this.getManagedProperties(guid)}", guid); err != nil {
		return nil, errors.Wrap(err, "failed to call cros_network_config javascript wrapper")
	}

	if result == nil {
		return nil, ErrNetworkNotFound
	}

	return result, nil
}

// ConfigureNetwork either configures a new network or updates an existing
// network configuration.
func (c *CrosNetworkConfig) ConfigureNetwork(ctx context.Context, properties types.ConfigProperties, shared bool) (string, error) {
	var result struct {
		GUID         string
		ErrorMessage string
	}

	if err := c.mojoRemote.Call(ctx, &result,
		"function(properties, shared) {return this.configureNetwork(properties, shared)}", properties, shared); err != nil {
		return "", errors.Wrap(err, "failed to call cros_network_config javascript wrapper")
	}

	if result.ErrorMessage != "" {
		return "", errors.New(result.ErrorMessage)
	}

	return result.GUID, nil
}

// ForgetNetwork removes the network with guid from the device.
func (c *CrosNetworkConfig) ForgetNetwork(ctx context.Context, guid string) (bool, error) {
	var result struct{ Success bool }
	if err := c.mojoRemote.Call(ctx, &result,
		"function(guid) {return this.forgetNetwork(guid)}", guid); err != nil {
		return false, errors.Wrap(err, "failed to run forgetNetwork")
	}
	return result.Success, nil
}

// SetNetworkTypeEnabledState enables/disable a given Network_Type.
func (c *CrosNetworkConfig) SetNetworkTypeEnabledState(ctx context.Context, networkType types.NetworkType, enable bool) error {
	var result struct{ Success bool }
	if err := c.mojoRemote.Call(ctx, &result,
		"function(networkType, enable) { return this.setNetworkTypeEnabledState(networkType, enable)}", networkType, enable); err != nil {
		return errors.Wrap(err, "failed to run setNetworkTypeEnabledState")
	}
	if result.Success != true {
		return errors.New("setNetworkTypeEnabledState failed")
	}

	return nil
}

// GetNetworkStateList returns a array of states of networks based on the filter.
func (c *CrosNetworkConfig) GetNetworkStateList(ctx context.Context, filter types.NetworkFilter) ([]types.NetworkStateProperties, error) {
	var result struct {
		Result []types.NetworkStateProperties
	}

	if err := c.mojoRemote.Call(ctx, &result,
		"function(filter) { return this.getNetworkStateList(filter)}", filter); err != nil {
		return result.Result, errors.Wrap(err, "failed to run getNetworkStateList")
	}

	return result.Result, nil
}

// GetDeviceStateList returns a array of Device states.
func (c *CrosNetworkConfig) GetDeviceStateList(ctx context.Context) ([]types.DeviceStateProperties, error) {
	var result struct{ Result []types.DeviceStateProperties }

	if err := c.mojoRemote.Call(ctx, &result,
		"function(filter) { return this.getDeviceStateList()}"); err != nil {
		return result.Result, errors.Wrap(err, "failed to run DeviceStateList")
	}

	return result.Result, nil
}

// CellularDeviceInhibitReason returns the current reason that the cellular device is inhibited, if any.
func (c *CrosNetworkConfig) CellularDeviceInhibitReason(ctx context.Context) (types.InhibitReason, error) {
	deviceStateList, err := c.GetDeviceStateList(ctx)
	if err != nil {
		return types.NotInhibited, errors.Wrap(err, "failed to get device state list")
	}

	cellularDevice := DeviceForNetworkType(deviceStateList, types.Cellular)
	if cellularDevice == nil {
		return types.NotInhibited, errors.New("failed to find cellular device")
	}
	return cellularDevice.InhibitReason, nil
}

func (c *CrosNetworkConfig) waitForCellularDeviceInhibitReason(ctx context.Context, condition func(types.InhibitReason) error) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		inhibitReason, err := c.CellularDeviceInhibitReason(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}

		sampleInterval := 3
		sampleTimeout := 15
		nthToSuccess := sampleTimeout / sampleInterval
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err = condition(inhibitReason); err != nil {
				return err
			}
			nthToSuccess--
			if nthToSuccess > 0 {
				return errors.Errorf("%d samples with stable inhibit reason", nthToSuccess)
			}
			return nil
		}, &testing.PollOptions{Timeout: time.Second * time.Duration(sampleTimeout), Interval: time.Second * time.Duration(sampleInterval)}); err != nil {
			return errors.Wrap(err, "failed to wait for a stable inhibit reason")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute * 8, Interval: time.Second * 15}); err != nil {
		return errors.Wrap(err, "failed to wait for cellular device inhibit reason condition to be met")
	}
	return nil
}

// WaitForCellularDeviceUninhibited waits until the cellular device is no longer inhibited,
// returning an error if the timeout has been reached.
func (c *CrosNetworkConfig) WaitForCellularDeviceUninhibited(ctx context.Context) error {
	return c.waitForCellularDeviceInhibitReason(ctx, func(inhibitReason types.InhibitReason) error {
		if inhibitReason != types.NotInhibited {
			return errors.Errorf("unexpected cellular network inhibit reason = got %v, want %v", inhibitReason, types.NotInhibited)
		}
		return nil
	})
}

// WaitForCellularDeviceInhibited waits until the cellular device is inhibited,
// returning an error if the timeout has been reached.
func (c *CrosNetworkConfig) WaitForCellularDeviceInhibited(ctx context.Context) error {
	return c.waitForCellularDeviceInhibitReason(ctx, func(inhibitReason types.InhibitReason) error {
		if inhibitReason == types.NotInhibited {
			return errors.New("cellular device is not inhibited")
		}
		return nil
	})
}
