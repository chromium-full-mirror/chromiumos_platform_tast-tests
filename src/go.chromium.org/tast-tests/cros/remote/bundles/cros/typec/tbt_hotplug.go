// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TbtHotplug,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Check that a Thunderbolt (3 or 4) device enumerates successfully on hotplug",
		Contacts:     []string{"chromeos-usb@google.com", "pmalani@chromium.org"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		SoftwareDeps: []string{"tpm2", "chrome"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Data:         []string{"testcert.p12"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort"},
	})
}

// TbtHotplug does the following:
//
// - Disconnect the dock via MCCI switch.
// - Verify that there is no Thunderbolt dock present on the system.
// - Log in with Peripheral Data Access Protection disabled.
// - Reconnect the dock via MCCI switch.
// - Verify that the Thunderbolt dock enumerates correctly.
//
// This test expects the following hardware topology:
//
//         - network -
//        /           \
//       /             \
//	Host -------- DUT ----- MCCI (`portUsed`) ---- Thunderbolt3 or Thunderbolt4 dock.
//	 |                              |
//	 |______________________________|
func TbtHotplug(ctx context.Context, s *testing.State) {
	numIterations := 10

	d := s.DUT()

	portUsed, err := strconv.Atoi(s.RequiredVar("typec.McciPort"))
	if err != nil {
		s.Fatal("Failed to parse MCCI port commandline variable: ", err)
	}

	sw, err := mcci.GetSwitch(s.RequiredVar("typec.McciSerial"))
	if err != nil {
		s.Fatal("Failed to get MCCI switch handle: ", err)
	}
	defer sw.Close()

	if err := typecutils.LoginChrome(ctx, d, s, "testcert.p12"); err != nil {
		s.Fatal("Failed to log in to Chrome: ", err)
	}

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performHotplugIteration(ctx, d, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}

		// GoBigSleepLint: Give enough time between iterations.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			s.Fatal("Failed to sleep between iterations: ", err)
		}
	}
}

// performHotplugIteration runs 1 iteration of the hotplug test.
func performHotplugIteration(ctx context.Context, d *dut.DUT, sw *mcci.Switch, mcciPort int) error {
	// Disconnect the dock.
	sw.DisablePorts()

	// Verify that there is no TBT device.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typecutils.CheckTBTDevice(ctx, d, false, typecutils.TbtGenAny)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20*time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT absence check")
	}

	// GoBigSleepLint: Give enough time between unplug -> plug.
	if err := testing.Sleep(ctx, 4*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep between unplug and plug")
	}

	// Reconnect the dock.
	sw.EnablePort(mcciPort)

	// Verify that there is a TBT device present.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typecutils.CheckTBTDevice(ctx, d, true, typecutils.TbtGenAny)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20*time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT presence check")
	}

	return nil
}
