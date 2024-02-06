// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	types "go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           MojoEthernetPreferredOverCellular,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Confirm Ethernet is preferred over cellular when both are enabled and wifi is disabled",
		Contacts:       []string{"cros-network-health-team@google.com", "chromeos-cellular-team@google.com", "shijinabraham@google.com"},
		BugComponent:   "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps:   []string{"chrome"},
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:        "cellularE2ELocal",
		Timeout:        10 * time.Minute,
	})
}

// MojoEthernetPreferredOverCellular checks that ethernet is preferred over
// cellular when both are enabled and connected with WiFi disabled.
// Todo(b:222693784) Add test to check whether Wifi is preferred over Cellular.
func MojoEthernetPreferredOverCellular(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed start Chrome: ", err)
	}
	defer cr.Close(ctx)

	// Enable cellular and connect to a network.
	cellularHelper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	service, err := cellularHelper.FindServiceForDevice(ctx)
	if err != nil {
		s.Fatal("Unable to find Cellular Service for Device: ", err)
	}
	if isConnected, err := service.IsConnected(ctx); err != nil {
		s.Fatal("Unable to get IsConnected for Service: ", err)
	} else if !isConnected {
		if _, err := cellularHelper.ConnectToDefault(ctx); err != nil {
			s.Fatal("Unable to Connect to Service: ", err)
		}
	}

	// Disable WiFi if present and defer re-enabling.
	if enableFunc, err := cellularHelper.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyWifi); err != nil {
		s.Fatal("Unable to disable WiFi: ", err)
	} else if enableFunc != nil {
		newCtx, cancel := ctxutil.Shorten(ctx, shill.EnableWaitTime)
		defer cancel()
		defer enableFunc(ctx)
		ctx = newCtx
	}

	// Lab devices will have ethernet already enabled and connected.

	netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(ctx)

	// Get state of all active networks.
	filter := types.NetworkFilter{
		Filter:      types.ActiveFT,
		NetworkType: types.All,
		Limit:       0}

	networkStates, err := netConn.GetNetworkStateList(ctx, filter)
	if err != nil {
		s.Fatal("Failed to get NetworkStateList: ", err)
	}

	// WiFi is disabled while cellular and ethernet is enabled and connected.
	// Since GetNetworkStateList returns networks in order of priority,
	// the first one should be ethernet and second one should be cellular.

	if len(networkStates) < 2 {
		s.Logf("NetworkStateList is %+v", networkStates)
		s.Fatal("Less than 2 networks in networkstatelist")
	}
	if networkStates[0].Type != types.Ethernet {
		s.Fatal("Wrong network in the second position expected: ethernet, got: ", networkStates[0].Type)
	}
	if !netconfig.NetworkStateIsConnectedOrOnline(networkStates[0]) {
		s.Fatal("Ethernet not Online or Connected, got: ", networkStates[0].ConnectionState)
	}
	if networkStates[1].Type != types.Cellular {
		s.Fatal("Wrong network in the second position expected: cellular, got: ", networkStates[1].Type)
	}
	if !netconfig.NetworkStateIsConnectedOrOnline(networkStates[1]) {
		s.Fatal("Cellular not Online or Connected, got: ", networkStates[1].ConnectionState)
	}
}
