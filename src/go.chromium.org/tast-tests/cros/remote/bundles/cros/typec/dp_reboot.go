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
		Func:     DpReboot,
		Desc:     "Check that a DisplayPort display enumerates successfully after reboot",
		Contacts: []string{"chromeos-usb@google.com", "pmalani@chromium.org"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_dp_bringup"},
		SoftwareDeps: []string{"reboot"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort"},
		Timeout:      6 * time.Minute,
	})
}

// DpReboot does the following:
//
// - Disconnect the DP dock/display via MCCI switch.
// - Connect the DP dock/display via MCCI switch.
// - Verify that the DP display enumerates correctly.
// - Reboot the system.
// - Verify that the DP display enumerates correctly.
//
// This test expects the following hardware topology:
//
//       - network -
//      /           \
//     /             \
//     Host -------- DUT ----- MCCI (`portUsed`) ---- DP display (can be connected via DP Type-C dock).
//     |                              |
//     |______________________________|
func DpReboot(ctx context.Context, s *testing.State) {
	const numIterations = 5

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
		if err := performDpRebootIteration(ctx, d, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performDpRebootIteration runs 1 iteration of the DisplayPort reboot test.
func performDpRebootIteration(ctx context.Context, d *dut.DUT, sw *mcci.Switch, mcciPort int) error {
	// Disconnect the dock/display.
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

	// Connect the dock/display.
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
		return errors.Wrap(err, "failed DP presence check before reboot")
	}

	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	// Verify DUT reconnected.
	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return errors.Wrap(err, "failed to re-connect to DUT after reboot")
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
		return errors.Wrap(err, "failed DP presence check after reboot")
	}

	return nil
}
