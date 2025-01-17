// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bluetoothutil provides common functions used by bluetooth test.
package bluetoothutil

const (
	// AudioPower is usually around 1.5W. Some models go up to 2.5W. Set the default limit to 2.7W.
	AudioPower = 2.7
	// IdlePower is set to 50mW to account for test variation.
	IdlePower = 0.05
	// IdleWith1PeerPower is power with 1 peer connected. It is set to 100mW.
	IdleWith1PeerPower = 0.1
	// IdleWithUIPower is set to 100mW due to larger test variation with UI.
	IdleWithUIPower = 0.1
	// ActiveDiscoveryPower the default limit is set to 0.6W.
	ActiveDiscoveryPower = 0.6
	// PassiveScanPower the default limit is set to 0.03W.
	PassiveScanPower = 0.03
	// FastPairDiscoverPower is the power when there is a nearby device compared to idle.
	FastPairDiscoverPower = 0.05
)
