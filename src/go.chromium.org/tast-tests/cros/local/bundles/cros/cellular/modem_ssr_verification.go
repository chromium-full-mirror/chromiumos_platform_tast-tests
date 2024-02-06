// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ModemSSRVerification,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that the modem is accessible after reset",
		Contacts:       []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_sim_active"},
		Fixture:        "cellularStressLocal",
		HardwareDeps:   hwdep.D(hwdep.CellularModemType(cellularconst.ModemTypeSC7180, cellularconst.ModemTypeSC7280)),
		Timeout:        3 * time.Minute,
	})
}

// ModemSSRVerification checks if the modem is accessible after reset.
func ModemSSRVerification(ctx context.Context, s *testing.State) {
	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	// Reset modem using shill to trigger SSR
	if err := helper.Device.Reset(ctx); err != nil {
		s.Fatal("Failed to reset the modem: ", err)
	}

	// Check if the Cellular Service was recreated.
	if _, err := helper.FindService(ctx); err != nil {
		s.Fatal("Unable to find Cellular Service: ", err)
	}
}
