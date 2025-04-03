// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typectest"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Tbt4Reboot,
		Desc:     "Check that a Thunderbolt 4 device enumerates successfully after reboot",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		SoftwareDeps: []string{"reboot"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort", "typec.McciPath"},
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_tbt4_bringup"},
			Val:       5,
			Timeout:   10 * time.Minute,
		}, {
			Name:    "stress",
			Val:     25,
			Timeout: 50 * time.Minute,
		}},
	})
}

// Tbt4Reboot does the following:
//
// - Disconnect the dock via MCCI switch.
// - Verify that there is no Thunderbolt 4 dock present on the system.
// - Reconnect the dock via MCCI switch.
// - Reboot the system.
// - Verify that the Thunderbolt 4 dock enumerates correctly.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host -------- DUT ----- MCCI (`portUsed`) ---- Thunderbolt 4 dock.
//	|                              |
//	|______________________________|
func Tbt4Reboot(ctx context.Context, s *testing.State) {

	numIterations := s.Param().(int)
	d := s.DUT()

	s.Log("Number of iterations: ", numIterations)

	portUsed, err := strconv.Atoi(s.RequiredVar("typec.McciPort"))
	if err != nil {
		s.Fatal("Failed to parse MCCI port commandline variable: ", err)
	}

	path, _ := s.Var("typec.McciPath")
	sw, err := mcci.GetSwitch(s.RequiredVar("typec.McciSerial"), path, portUsed)
	if err != nil {
		s.Fatal("Failed to get MCCI switch handle: ", err)
	}
	defer sw.Close(ctx)

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performTbt4RebootIteration(ctx, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}

		// GoBigSleepLint: Give enough time between iterations.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			s.Fatal("Failed to sleep between iterations: ", err)
		}
	}
}

// performTbt4RebootIteration runs 1 iteration of the Thunderbolt 4 reboot test.
func performTbt4RebootIteration(ctx context.Context, d *dut.DUT, sw *mcci.Switch) error {
	// Disconnect the dock.
	sw.DisablePorts(ctx)

	// Verify that there is no TBT device.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typectest.CheckTBTDevice(ctx, d, false, typectest.TbtGenAny)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT4 absence check")
	}

	// GoBigSleepLint: Give enough time between unplug -> plug.
	if err := testing.Sleep(ctx, 4*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep after unplugging Type-C port")
	}

	// Reconnect the dock.
	sw.EnablePort(ctx)

	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	// Verify DUT reconnected.
	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return errors.Wrap(err, "failed to re-connect to DUT after reboot")
	}

	// Verify that there is a TBT device present.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typectest.CheckTBTDevice(ctx, d, true, typectest.TbtGen4)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT4 presence check")
	}

	return nil
}
