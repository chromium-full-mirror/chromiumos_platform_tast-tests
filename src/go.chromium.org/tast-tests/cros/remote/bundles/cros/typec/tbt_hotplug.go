// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/usbutils/usbswitch"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typectest"
	"go.chromium.org/tast-tests/cros/remote/typec/typecswitch"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     TbtHotplug,
		Desc:     "Check that a Thunderbolt (3 or 4) device enumerates successfully on hotplug",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		SoftwareDeps: []string{"tpm2", "chrome"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Data:         []string{"testcert.p12"},
		Fixture:      "typecSwitch",
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_tbt4_bringup", "typec_tbt3_bringup"},
			Val: typecswitch.TestSetupData{
				ConnectionMode: usbswitch.TBT4Mode,
				Iterations:     10,
			},
			Timeout: 10 * time.Minute,
		}, {
			Name: "stress",
			Val: typecswitch.TestSetupData{
				ConnectionMode: usbswitch.TBT4Mode,
				Iterations:     50,
			},
			Timeout: 50 * time.Minute,
		}},
	})
}

// TbtHotplug does the following:
//
// - Disconnect the dock via USB switch.
// - Verify that there is no Thunderbolt dock present on the system.
// - Log in with Peripheral Data Access Protection disabled.
// - Reconnect the dock via USB switch.
// - Verify that the Thunderbolt dock enumerates correctly.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch ---- Thunderbolt3 or Thunderbolt4 dock.
//	|                            |
//	|____________________________|
func TbtHotplug(ctx context.Context, s *testing.State) {
	d := s.DUT()
	testData := s.Param().(typecswitch.TestSetupData)
	s.Log("Number of iterations: ", testData.Iterations)

	// Get the switch from the fixture.
	fixtData, ok := s.FixtValue().(*typecswitch.FixtureData)
	if !ok {
		s.Fatal("Failed to get fixture data")
	}
	sw := fixtData.TestSwitch

	if err := typecswitch.SetupSwitch(ctx, sw, testData); err != nil {
		s.Fatal("Failed to setup switch: ", err)
	}

	if err := typectest.LoginChrome(ctx, d, s, "testcert.p12"); err != nil {
		s.Fatal("Failed to log in to Chrome: ", err)
	}

	for i := 1; i <= testData.Iterations; i++ {
		s.Log("Running iteration ", i)
		if err := performHotplugIteration(ctx, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
		// GoBigSleepLint: Give enough time between iterations.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			s.Fatal("Failed to sleep between iterations: ", err)
		}
	}
}

// performHotplugIteration runs 1 iteration of the hotplug test.
func performHotplugIteration(ctx context.Context, d *dut.DUT, sw usbswitch.Switch) error {
	// Disconnect the dock.
	sw.DisablePorts(ctx)

	// Verify that there is no TBT device.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typectest.CheckTBTDevice(ctx, d, false, typectest.TbtGenAny)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT absence check")
	}

	// GoBigSleepLint: Give enough time between unplug -> plug.
	if err := testing.Sleep(ctx, 4*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep between unplug and plug")
	}

	// Reconnect the dock.
	sw.EnablePort(ctx)

	// Verify that there is a TBT device present.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typectest.CheckTBTDevice(ctx, d, true, typectest.TbtGenAny)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT presence check")
	}

	return nil
}
