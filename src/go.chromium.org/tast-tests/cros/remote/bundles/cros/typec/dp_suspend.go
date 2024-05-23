// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DpSuspend,
		Desc:     "Check that a DisplayPort display enumerates before and after suspend",
		Contacts: []string{"chromeos-usb-champs@google.com", "pmalani@chromium.org"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_dp_bringup"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort"},
		Timeout:      6 * time.Minute,
	})
}

// DpSuspend does the following:
//
// - Disconnect the DP dock/display via MCCI switch.
// - Verify that there is no DP display present on the system.
// - Reconnect the dock/display via MCCI switch.
// - Verify that the DP display enumerates correctly.
// - Suspend the DUT. Wait for it to wake up.
// - Verify that the DP display enumerates correctly.
//
// This test expects the following hardware topology:
//
//        - network -
//       /           \
//      /             \
//     Host -------- DUT ----- MCCI (`portUsed`) ---- DP display (can be connected via DP Type-C dock).
//      |                              |
//      |______________________________|
func DpSuspend(ctx context.Context, s *testing.State) {
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

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performDpSuspendIteration(ctx, d, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performDpSuspendIteration runs 1 iteration of the DP suspend test.
func performDpSuspendIteration(ctx context.Context, d *dut.DUT, sw *mcci.Switch, mcciPort int) error {
	const suspendDurationS = 10

	// Disconnect the dock/display.
	sw.DisablePorts()

	// Verify that there is no DP display.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if conns, err := typecutils.FindConnectedDp(ctx, d); err != nil {
			return err
		} else if len(conns) != 0 {
			return errors.Errorf("found connected DP connectors: %s", strings.Join(conns, " "))
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed DP absence check")
	}

	// Reconnect the dock/display.
	sw.EnablePort(mcciPort)

	// Verify that there is a DP display.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if conns, err := typecutils.FindConnectedDp(ctx, d); err != nil {
			return err
		} else if len(conns) == 0 {
			return errors.New("no connected DP connectors found")
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed DP presence check before suspend")
	}

	// GoBigSleepLint: If we sleep immediately, power_manager nixes the attempt to enter suspend. So wait for a little while.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for display unplug modeset")
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

	// Verify that there is a DP display.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if conns, err := typecutils.FindConnectedDp(ctx, d); err != nil {
			return err
		} else if len(conns) == 0 {
			return errors.New("no connected DP connectors found")
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed DP presence check after suspend")
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
