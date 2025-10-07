// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/common/usbutils/usbswitch"
	"go.chromium.org/tast-tests/cros/remote/typec/typecswitch"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ACSupplyHotplug,
		Desc:     "Check that a charger is properly connected and disconnected by the DUT",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_utc274"},
		Fixture:      "typecSwitchAndServo",
		Params: []testing.Param{{
			Name: "normal",
			Val: typecswitch.TestSetupData{
				ConnectionMode: usbswitch.Usb3Mode,
				Iterations:     300,
			},
			Timeout: 150 * time.Minute,
		}, {
			Name: "flipped",
			Val: typecswitch.TestSetupData{
				ConnectionMode: usbswitch.Usb3Mode,
				Iterations:     300,
				Flipped:        true,
			},
			Timeout: 150 * time.Minute,
		}},
	})
}

// ACSupplyHotplug does the following:
//
// - Disconnect the AC supply via switch interface.
// - Verify that the AC supply is disconnected.
// - Reconnect the AC supply via switch interface.
// - Verify that the AC supply is connected.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB Switch  ---- AC Supply
//	|                            |
//	|____________________________|
func ACSupplyHotplug(ctx context.Context, s *testing.State) {
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

	for i := 1; i <= testData.Iterations; i++ {
		s.Log("Running iteration ", i)
		if err := performACSupplyHotplugIteration(ctx, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performACSupplyHotplugIteration runs 1 iteration of the AC supply hotplug test.
func performACSupplyHotplugIteration(ctx context.Context, d *dut.DUT, sw usbswitch.Switch) error {
	// Enable the port
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to enable the port")
	}

	// Check for connection
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		connected, err := typecutils.VerifyChargerConnected(ctx, d)
		if err != nil {
			return errors.Wrap(err, "failed to verify charger connection")
		}
		if !connected {
			return errors.New("charger is not connected after enabling the port")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		return err
	}

	// Disable the port
	if err := sw.DisablePorts(ctx); err != nil {
		return errors.Wrap(err, "failed to disable the port")
	}

	// Check for disconnection
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		connected, err := typecutils.VerifyChargerConnected(ctx, d)
		if err != nil {
			return errors.Wrap(err, "failed to verify charger connection")
		}
		if connected {
			return errors.New("charger is still connected after disabling the port")
		}
		return nil
	}, &testing.PollOptions{Timeout: 3000 * time.Millisecond, Interval: 100 * time.Millisecond}); err != nil {
		return err
	}

	return nil
}
