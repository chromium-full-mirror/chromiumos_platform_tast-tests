// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package netconfig

import (
	"github.com/godbus/dbus/v5"

	types "go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast/core/errors"
)

// This file contains helper functions for commonly used patterns that occur in connectivity tests using mojo.

// NetworkStateIsConnectedOrOnline checks whether network is connected or online.
func NetworkStateIsConnectedOrOnline(networkState types.NetworkStateProperties) bool {
	return networkState.ConnectionState == types.ConnectedCST || networkState.ConnectionState == types.OnlineCST
}

// GetBoolValue returns a bool indexed by key.
func GetBoolValue(m map[string]dbus.Variant, key string) (bool, error) {
	variant, ok := m[key]
	if !ok {
		return false, errors.Errorf("failed to get variant from map %v for key %v", variant, key)
	}
	val, ok := variant.Value().(bool)
	if !ok {
		return false, errors.Errorf("failed to get bool value from dbus.Variant %v", variant)
	}
	return val, nil
}

// GetStringValue returns a string indexed by key.
func GetStringValue(m map[string]dbus.Variant, key string) (string, error) {
	variant, ok := m[key]
	if !ok {
		return "", errors.Errorf("failed to get variant from map %v for key %v", variant, key)
	}
	val, ok := variant.Value().(string)
	if !ok {
		return "", errors.Errorf("failed to get string value from dbus.Variant %v", variant)
	}
	return val, nil
}

// DeviceForNetworkType returns the device for the given network type.
func DeviceForNetworkType(deviceStateList []types.DeviceStateProperties, networkType types.NetworkType) *types.DeviceStateProperties {
	for _, d := range deviceStateList {
		if d.Type == networkType {
			return &d
		}
	}
	return nil
}
