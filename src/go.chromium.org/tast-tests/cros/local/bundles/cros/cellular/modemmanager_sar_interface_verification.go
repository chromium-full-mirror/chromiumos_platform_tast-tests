// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModemmanagerSARInterfaceVerification,
		Desc:         "Verifies that modemmanager SAR interface enable, disable succeeds",
		Contacts:     []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active"},
		HardwareDeps: hwdep.D(hwdep.CellularSoftwareDynamicSar()),
		Fixture:      "cellularModemManager",
		Timeout:      5 * time.Minute,
	})
}

// ModemmanagerSARInterfaceVerification Test
func ModemmanagerSARInterfaceVerification(ctx context.Context, s *testing.State) {
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim (precondition): ", err)
	}

	if err := modem.Enable(ctx); err != nil {
		s.Fatal("Modem enable failed with: ", err)
	}

	sar, err := modem.GetSARInterface(ctx)
	if err != nil {
		s.Fatal("Failed to read SAR interface: ", err)
	}

	// Check Enable SAR
	if err := updateAndCheckSARState(ctx, sar, true); err != nil {
		s.Fatal("Failed to enable SAR: ", err)
	} else {
		s.Log("Enabled SAR")
	}

	// Check Disable SAR
	if err := updateAndCheckSARState(ctx, sar, false); err != nil {
		s.Fatal("Failed to disable SAR: ", err)
	} else {
		s.Log("Disabled SAR")
	}

	// Re-enable SAR
	if err := updateAndCheckSARState(ctx, sar, true); err != nil {
		s.Fatal("Failed to re-enable SAR: ", err)
	} else {
		s.Log("Re-enabled SAR")
	}

}

func updateAndCheckSARState(ctx context.Context, modem *modemmanager.Modem, state bool) error {
	if err := modem.EnableSAR(ctx, state); err != nil {
		return errors.Wrap(err, "failed to call EnableSAR()")
	}

	enabled, err := modem.IsSAREnabled(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read SARState")
	}
	testing.ContextLog(ctx, "SARState :", enabled)

	if enabled != state {
		return errors.New("failed to set SAR State")
	}

	return nil
}
