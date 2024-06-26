// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/networkui/netconfig"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MojoCellularToggle,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Enable/disable Cellular service using Mojo and confirms using shill",
		Contacts: []string{
			"chromeos-connectivity-cienet-external@google.com",
			"alfred.yu@cienet.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_sim_active"},
		Timeout:      10 * time.Minute,
		Fixture:      "cellularWithChrome",
	})
}

// MojoCellularToggle enables/distable cellular network using Mojo and confirms using shill helper
func MojoCellularToggle(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*cellular.FixtData).Chrome

	helper := s.FixtValue().(*cellular.FixtData).Helper

	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Failed to enable cellular: ", err)
	}

	netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(ctx)

	const iterations = 5
	for i := 0; i < iterations; i++ {
		enabled := i%2 != 0
		s.Logf("Toggling Cellular state to %t (iteration %d of %d)", enabled, i+1, iterations)

		if err := netConn.SetNetworkTypeEnabledState(ctx, netconfigtypes.Cellular, enabled); err != nil {
			s.Fatal("Failed to set cellular state: ", err)
		}

		if err := helper.WaitForEnabledState(ctx, enabled); err != nil {
			s.Fatal("cellular state is not as expected: ", err)
		}
	}
}
