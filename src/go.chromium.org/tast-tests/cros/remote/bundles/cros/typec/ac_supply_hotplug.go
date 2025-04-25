// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecswitch"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ACSupplyHotplug,
		Desc:     "Check that a charger can be connected and disconnected using the switch interface",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_unigraf274"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort", "typec.McciPath", "typec.UnigrafUri", "servo"},
		Params: []testing.Param{{
			Val:     300,
			Timeout: 30 * time.Minute,
		}},
	})
}

// ACSupplyHotplug does the following:
//
// - Disconnect the AC supply via switch interface.
// - Reconnect the AC supply via switch interface.
// - Verify that the AC supply is connected.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host -------- DUT ----- Switch (`portUsed`) ---- AC Supply
//	|                              |
//	|______________________________|
func ACSupplyHotplug(ctx context.Context, s *testing.State) {
	numIterations := s.Param().(int)
	d := s.DUT()

	s.Log("Number of iterations: ", numIterations)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	sw, err := typecswitch.GetSwitch(ctx, s)
	if err != nil {
		s.Fatal("Failed to get switch handle: ", err)
	}
	defer sw.Close(cleanupCtx)

	// If servo is present, make sure it's in snk role.
	if servoSpec, present := s.Var("servo"); present {
		pxy, err := servo.NewProxy(ctx, servoSpec, d.KeyFile(), d.KeyDir())
		if err != nil {
			s.Fatal("Failed to setup servo proxy: ", err)
		}
		if err := pxy.Servo().ServoCcSnk(ctx); err != nil {
			s.Fatal("Failed to set servo CC to off: ", err)
		}
		defer pxy.Servo().ServoCcSrc(cleanupCtx, true)

		// On Unigraf setup, ethernet is connected by servo, wait for the connection to resume.
		connectCtx, connectCtxCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := d.WaitConnect(connectCtx); err != nil {
			s.Fatal("DUT not reachable in time: ", err)
		}
		connectCtxCancel()
	}

	// Make sure the AC supply is disconnected before testing
	testPort, err := sw.TestPort(ctx)
	if err != nil {
		s.Fatal("Could not get active port before testing")
	}

	if devicePort, err := sw.DevicePort(ctx); err != nil {
		s.Fatal("Could not get used port before testing: ", err)
	} else if devicePort == testPort {
		if err := sw.DisablePorts(ctx); err != nil {
			s.Fatal("Could not disable the port before testing: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			connected, err := typecutils.VerifyChargerConnected(ctx, d)
			if err != nil {
				return errors.Wrap(err, "failed to verify charger connection")
			}
			if connected {
				return errors.New("charger is still connected after disabling the port")
			}
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
			s.Fatal("Failed to disconnect the device before the test: ", err)
		}
	} else if err := sw.DisablePorts(ctx); err != nil {
		s.Fatal("Could not disable the port before testing: ", err)
	}

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performACSupplyHotplugIteration(ctx, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performACSupplyHotplugIteration runs 1 iteration of the AC supply hotplug test.
func performACSupplyHotplugIteration(ctx context.Context, d *dut.DUT, sw typecswitch.Switch) error {
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
	}, &testing.PollOptions{Timeout: 6000 * time.Millisecond, Interval: 500 * time.Millisecond}); err != nil {
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
