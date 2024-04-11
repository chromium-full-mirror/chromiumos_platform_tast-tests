// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	types "go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/networkui/netconfig"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           MojoEnsureNetworkStateAfterCellularToggle,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Enable/disable Cellular service using Mojo and check WiFi/Ethernet are not affected",
		Contacts:       []string{"cros-network-health-team@google.com", "chromeos-cellular-team@google.com", "shijinabraham@google.com"},
		BugComponent:   "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps:   []string{"chrome"},
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:        "cellularE2ELocal",
		Timeout:        10 * time.Minute,
	})
}

// compareDeviceStatelist compares the device states with states before toggling cellular device.
// cellular device state should match 'enabled' argument and Ethernet and WiFi should maintain the original state
func compareDeviceStatelist(orgDeviceState, curDeviceState []types.DeviceStateProperties, enabled bool) error {
	for _, cfg := range curDeviceState {
		deviceType := cfg.Type
		deviceState := cfg.DeviceState
		// Check cellular device against the known status.
		if deviceType == types.Cellular {
			if enabled && deviceState != types.EnabledDST {
				return errors.Errorf("unexpected Cellular state, got: %v, want: enabled", deviceState)

			} else if !enabled && deviceState != types.DisabledDST {
				return errors.Errorf("unexpected Cellular state, got: %v, want: disabled", deviceState)
			}
		}

		// Checking only Ethernet and WiFi.
		if deviceType != types.Ethernet && deviceType != types.WiFi {
			continue
		}

		// Ethernet and WiFi should maintain the original state.
		for _, cfg := range orgDeviceState {
			if cfg.Type == deviceType && cfg.DeviceState != deviceState {
				return errors.Errorf("unexpected state for %v device, got: %v, want: %v", cfg.Type, deviceState, cfg.DeviceState)
			}
		}
	}
	return nil
}

// MojoEnsureNetworkStateAfterCellularToggle enables/disables cellular device using Mojo and confirms using mojo that WiFi/Ethernet are not affected.
func MojoEnsureNetworkStateAfterCellularToggle(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed start Chrome: ", err)
	}
	defer cr.Close(ctx)

	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	// Enable Wifi.
	wifiManager, err := shill.NewWifiManager(ctx, nil)
	if err != nil {
		s.Fatal("Failed to create shill Wi-Fi manager: ", err)
	}
	if err := wifiManager.Enable(ctx, true); err != nil {
		s.Fatal("Failed to enable Wi-Fi: ", err)
	}

	netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(ctx)

	orgDeviceState, err := netConn.GetDeviceStateList(ctx)
	if err != nil {
		s.Fatal("Failed to get DeviceStateList: ", err)
	}

	const iterations = 5
	for i := 0; i < iterations; i++ {
		enabled := i%2 != 0
		s.Logf("Toggling Cellular state to %t (iteration %d of %d)", enabled, i+1, iterations)

		if err = netConn.SetNetworkTypeEnabledState(ctx, types.Cellular, enabled); err != nil {
			s.Fatal("Failed to set cellular state: ", err)
		}

		if err = helper.WaitForEnabledState(ctx, enabled); err != nil {
			s.Fatal("cellular state is not as expected: ", err)
		}

		curDeviceState, err := netConn.GetDeviceStateList(ctx)
		if err != nil {
			s.Fatal("Failed to get DeviceStateList: ", err)
		}
		if err := compareDeviceStatelist(orgDeviceState, curDeviceState, enabled); err != nil {
			s.Fatal("Device state comparison failed: ", err)
		}
	}
}
