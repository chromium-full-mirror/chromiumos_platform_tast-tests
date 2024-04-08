// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillSimChangePin,
		Desc:         "Verifies that the cellular device SIM PIN can be changed",
		Contacts:     []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_pinlock"},
		Fixture:      "cellularSIMLockCleared",
		Timeout:      5 * time.Minute,
	})
}

// ShillSimChangePin tests successfully changing SIM pin.
func ShillSimChangePin(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}
	currentPin, currentPuk, err := helper.GetPINAndPUKForICCID(ctx, iccid)
	if err != nil {
		s.Fatal("Could not get Pin and Puk : ", err)
	}
	if currentPuk == "" {
		s.Fatal("Unable to find PUK code for ICCID : ", iccid)
	}

	tempPin := mmconst.TempSimPin

	nboard, err := cellular.GetBoard(ctx)
	s.Log("Board name: ", nboard)
	// ResetModem/RestartModemWithHelper needed for sim power reset to reflect locked type values.
	if nboard == "zork" {
		//Zork failing with Device.Reset, modemfwd helper gpio reset works on zork boards - TBD: b/330944917.
		if _, err := cellular.RestartModemWithHelper(ctx); err != nil {
			s.Fatal("Failed to reset modem: ", err)
		}
	} else if _, err := helper.ResetModem(ctx); err != nil {
		s.Fatal("Failed to reset modem: ", err)
	}

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Test sequence :
	// Enable Default Pin => Change Pin to Temp Pin => Check TempPin lockstates =>
	// Revert to Default Pin => Disable Default Pin.

	// Enable pin lock.
	s.Log("Calling requirepin to enable pin lock")
	if err = helper.Device.RequirePin(ctx, currentPin, true); err != nil {
		s.Fatal("Failed to enable lock: ", err)
	}

	defer func(ctx context.Context) {
		// Unlock and disable pin lock.
		if err = helper.Device.RequirePin(ctx, currentPin, false); err != nil {
			s.Fatal("Failed to disable default pin lock: ", err)
		}
	}(cleanupCtx)

	s.Log("Attempting to change enabled sim lock with temporary pin: ", tempPin)
	if err = helper.Device.ChangePin(ctx, currentPin, tempPin); err != nil {
		s.Fatal("Failed to change pin: ", err)
	}

	enabled := false
	locked := false

	defer func(ctx context.Context) {
		if enabled || locked {
			// Reverse to default pin.
			s.Log("Reverse pin to default pin from temppin")
			if err = helper.Device.ChangePin(ctx, tempPin, currentPin); err != nil {
				s.Fatal("Failed to change pin lock to default pin lock: ", err)
			}
		}
	}(cleanupCtx)

	// ResetModem/RestartModemWithHelper needed for sim power reset to reflect locked type values.
	if nboard == "zork" {
		//Zork failing with Device.Reset, modemfwd helper gpio reset works on zork boards - TBD: b/330944917.
		if _, err := cellular.RestartModemWithHelper(ctx); err != nil {
			s.Fatal("Failed to reset modem: ", err)
		}
	} else if _, err := helper.ResetModem(ctx); err != nil {
		s.Fatal("Failed to reset modem: ", err)
	}

	enabled = helper.IsSimLockEnabled(ctx)
	if !enabled {
		s.Fatal("SIM lock not enabled by temp pin: ", err)
	}
	locked = helper.IsSimPinLocked(ctx)
	if !locked {
		s.Fatal("SIM lock was not locked by temp pin: ", err)
	}
}
