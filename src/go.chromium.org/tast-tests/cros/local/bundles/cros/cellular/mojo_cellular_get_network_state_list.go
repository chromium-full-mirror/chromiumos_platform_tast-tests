// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"math"
	"time"

	types "go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MojoCellularGetNetworkStateList,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensure that CrosNetworkConfig.GetNetworkStateList matches properties in Shill ",
		Contacts:     []string{"chromeos-cellular-team@google.com", "cros-network-health-team@google.com", "khegde@google.com"},
		BugComponent: "b:1166446",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e"},
		Timeout:      10 * time.Minute,
		Fixture:      "cellular",
	})
}

func MojoCellularGetNetworkStateList(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed start Chrome: ", err)
	}
	defer cr.Close(ctx)

	cellularHelper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(ctx)

	service, err := cellularHelper.Connect(ctx)
	if err != nil {
		s.Fatal("Failed to get a connected cellular service: ", err)
	}

	// Get state of all active cellular networks.
	filter := types.NetworkFilter{
		Filter:      types.ActiveFT,
		NetworkType: types.Cellular,
		Limit:       0}

	networkStates, err := netConn.GetNetworkStateList(ctx, filter)
	if err != nil {
		s.Fatal("Failed to get NetworkStateList: ", err)
	}

	if len(networkStates) != 1 {
		s.Fatal("Failed to get an active cellular network. Want: 1, Got: ", len(networkStates))
	}
	cellularNetwork := networkStates[0].TypeState.Cellular

	iccid, err := cellularHelper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Failed to get Iccid for service: ", err)
	}
	if iccid != cellularNetwork.Iccid {
		s.Fatal("Iccid mismatch: want: ", iccid, "got: ", cellularNetwork.Iccid)
	}

	eid, err := cellularHelper.GetCurrentEID(ctx)
	if err != nil {
		s.Fatal("Failed to get Eid for service: ", err)
	}
	if eid != cellularNetwork.Eid {
		s.Fatal("Eid mismatch: ", eid, cellularNetwork.Eid)
	}

	activationState, err := cellularHelper.GetCurrentActivationState(ctx)
	if err != nil {
		s.Fatal("Failed to get ActivationState for service: ", err)
	}
	actSt := types.ConvertActivationStateTypeToShillString(cellularNetwork.ActivationState)
	if activationState != actSt {
		s.Fatal("ActivationState mismatch: want: ", activationState, " got: ", actSt)
	}

	networkTechnology, err := cellularHelper.GetCurrentNetworkTechnology(ctx)
	if err != nil {
		s.Fatal("Failed to get NetworkTechnology for service: ", err)
	}
	if networkTechnology != cellularNetwork.NetworkTechnology {
		s.Fatal("NetworkTechnology mismatch: want: ", networkTechnology, " got: ", cellularNetwork.NetworkTechnology)
	}

	roamingState, err := cellularHelper.GetCurrentRoamingState(ctx)
	if err != nil {
		s.Fatal("Failed to get RoamingState for service: ", err)
	}
	providerRequiresRoaming, err := cellularHelper.GetProviderRequiresRoamingFromShill(ctx)
	if err != nil {
		s.Fatal("Failed to get ProviderRequiresRoaming for service: ", err)
	}
	// Note that cellularNetwork.Roaming means network == roaming &&
	// provider does not require roaming (see
	// chromium_src:chromeos/ash/components/network/network_state.cc). The
	// "roaming" string used below is from
	// third_party/cros_system_api/dbus/shill/dbus-constants.h.
	isRoaming := roamingState == "roaming" && !providerRequiresRoaming
	if isRoaming != cellularNetwork.Roaming {
		s.Fatal("RoamingState mismatch: want: ", roamingState, " got: ", cellularNetwork.Roaming)
	}

	// Since the signal strength could fluctuate between the two calls to
	// shill (one via the helper, other via cros_network_config), ensure
	// that the two values are within a threshold.
	if err != nil {
		s.Fatal("Unable to find Cellular Service for Device: ", err)
	}
	strength, err := service.GetSignalStrength(ctx)
	if err != nil {
		s.Fatal("Failed to get SignalStrength for service: ", err)
	}
	threshold := 10.0
	if math.Abs(float64(int32(strength)-cellularNetwork.SignalStrength)) > threshold {
		s.Fatal("SignalStrength not within threshold: want: ", strength, " got: ", cellularNetwork.SignalStrength)
	}

	simLockEnabled := cellularHelper.IsSimLockEnabled(ctx)
	if simLockEnabled != cellularNetwork.SimLocked {
		s.Fatal("SimLocked mismatch: want: ", simLockEnabled, " got: ", cellularNetwork.SimLocked)
	}
}
