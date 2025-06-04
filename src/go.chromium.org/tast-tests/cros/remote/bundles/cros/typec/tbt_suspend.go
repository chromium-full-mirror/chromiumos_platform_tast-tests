// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strconv"
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
		Func:     TbtSuspend,
		Desc:     "Check that a Thunderbolt (3 or 4) device enumerates before and after suspend",
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
			Val:       10,
			Timeout:   8 * time.Minute,
		}, {
			Name:    "stress",
			Val:     50,
			Timeout: 40 * time.Minute,
		}},
	})
}

// TbtSuspend does the following:
//
// - Disconnect the dock via USB switch.
// - Verify that there is no Thunderbolt dock present on the system.
// - Reconnect the dock via USB switch.
// - Verify that the Thunderbolt dock enumerates correctly.
// - Suspend the DUT with timeout.
// - Verify that the Thunderbolt dock still enumerates correctly.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch ---- Thunderbolt (3 or 4) dock.
//	|                            |
//	|____________________________|
func TbtSuspend(ctx context.Context, s *testing.State) {
	d := s.DUT()
	numIterations := s.Param().(int)
	s.Log("Number of iterations: ", numIterations)

	// Get the switch from the fixture.
	fixtData, ok := s.FixtValue().(*typecswitch.FixtureData)
	if !ok {
		s.Fatal("Failed to get fixture data")
	}
	sw := fixtData.TestSwitch

	if err := typectest.LoginChrome(ctx, d, s, "testcert.p12"); err != nil {
		s.Fatal("Failed to log in to Chrome: ", err)
	}

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performTbtSuspendIteration(ctx, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performTbtSuspendIteration runs 1 iteration of the Thunderbolt suspend test.
func performTbtSuspendIteration(ctx context.Context, d *dut.DUT, sw usbswitch.Switch) error {
	const suspendDurationS = 15

	// Disconnect the dock.
	sw.DisablePorts(ctx)

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

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typectest.CheckTBTDevice(ctx, d, true, typectest.TbtGenAny)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT presence check")
	}

	// Suspend DUT. Run in a separate thread since this function blocks until resume.
	done := make(chan error, 1)
	go func(ctx context.Context) {
		defer close(done)
		out, err := d.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--timeout=120", "--suspend_for_sec="+strconv.Itoa(suspendDurationS)).CombinedOutput()
		testing.ContextLog(ctx, "powerd_dbus_suspend output: ", string(out))
		done <- err
	}(ctx)

	susCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := d.WaitUnreachable(susCtx); err != nil {
		return errors.Wrap(err, "couldn't verify DUT became unreachable after suspend")
	}

	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to re-connect to DUT after suspend")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return typectest.CheckTBTDevice(ctx, d, true, typectest.TbtGenAny)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed TBT presence check")
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
