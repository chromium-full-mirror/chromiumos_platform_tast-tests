// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strings"
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
		Func:     Tbt4HotplugSuspend,
		Desc:     "Check that a Thunderbolt 4 device enumerates successfully on hotplug during suspend",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Fixture:      "typecSwitch",
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_tbt4_bringup"},
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

// Tbt4HotplugSuspend does the following:
//
// - Disconnect the dock via USB switch.
// - Verify that there is no Thunderbolt 4 dock present on the system.
// - Suspend the DUT.
// - Reconnect the dock via USB switch.
// - Verify that the Thunderbolt 4 dock enumerates correctly.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch ---- Thunderbolt 4 dock.
//	|                              |
//	|______________________________|
func Tbt4HotplugSuspend(ctx context.Context, s *testing.State) {
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
		if err := performHotplugSuspendIteration(ctx, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}

		// GoBigSleepLint: Give enough time between iterations.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			s.Fatal("Failed to sleep between iterations: ", err)
		}
	}
}

// performHotplugSuspendIteration runs 1 iteration of the hotplug in suspend test.
func performHotplugSuspendIteration(ctx context.Context, d *dut.DUT, sw usbswitch.Switch) error {
	// Disconnect the dock.
	sw.DisablePorts(ctx)

	// Verify that there is no TBT device.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typectest.CheckTBTDevice(ctx, d, false, typectest.TbtGenAny)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT4 absence check")
	}

	// Suspend DUT. Run in a separate thread since this function blocks until resume.
	done := make(chan error, 1)
	go func(ctx context.Context) {
		defer close(done)
		out, err := d.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--timeout=120").CombinedOutput()
		testing.ContextLog(ctx, "powerd_dbus_suspend output: ", string(out))
		done <- err
	}(ctx)

	susCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := d.WaitUnreachable(susCtx); err != nil {
		return errors.Wrap(err, "couldn't verify DUT became unreachable after suspend")
	}

	// GoBigSleepLint: Wait a second in suspend.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep while DUT is in suspend")
	}

	// Reconnect the dock.
	sw.EnablePort(ctx)

	// Verify DUT reconnected.
	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to re-connect to DUT after suspend")
	}

	// Verify that there is a TBT device present.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typectest.CheckTBTDevice(ctx, d, true, typectest.TbtGen4)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT4 presence check")
	}

	// Verify that the suspend command didn't return an unexpected error.
	if err := <-done; err != nil {
		const errNoExitStatus = "remote command exited without exit status"
		if !strings.Contains(err.Error(), errNoExitStatus) {
			return errors.Wrap(err, "suspend command returned unexpected error")
		}
	}

	return nil
}
