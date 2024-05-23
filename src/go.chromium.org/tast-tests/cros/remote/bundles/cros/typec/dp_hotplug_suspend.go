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
		Func:     DpHotplugSuspend,
		Desc:     "Check that a DisplayPort display enumerates successfully on hotplug during suspend",
		Contacts: []string{"chromeos-usb-champs@google.com", "pmalani@chromium.org"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_dp_bringup"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort"},
		Timeout:      6 * time.Minute,
	})
}

// DpHotplugSuspend does the following:
//
// - Disconnect the DP display via MCCI switch.
// - Verify that there is no DP display present on the system.
// - Suspend the DUT.
// - Reconnect the DP display via MCCI switch, while DUT is in suspend.
// - Verify that the DP display enumerates correctly after resume.
//
// This test expects the following hardware topology:
//
//       - network -
//      /           \
//     /             \
//     Host -------- DUT ----- MCCI (`portUsed`) ---- DP display (can be connected via DP Type-C dock).
//     |                              |
//     |______________________________|
func DpHotplugSuspend(ctx context.Context, s *testing.State) {
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
		if err := performDpHotplugSuspendIteration(ctx, d, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performDpHotplugSuspendIteration runs 1 iteration of the hotplug in suspend test.
func performDpHotplugSuspendIteration(ctx context.Context, d *dut.DUT, sw *mcci.Switch, mcciPort int) error {
	const suspendDurationS = 10

	// Disconnect the dock.
	sw.DisablePorts()

	// GoBigSleepLint: Give enough time for a new display modeset after hot unplug.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for display unplug modeset")
	}

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

	// GoBigSleepLint: Wait a second in suspend.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep while DUT is in suspend")
	}

	// Reconnect the dock/display.
	sw.EnablePort(mcciPort)

	// Verify DUT reconnected.
	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: 2 * suspendDurationS * time.Second}); err != nil {
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
