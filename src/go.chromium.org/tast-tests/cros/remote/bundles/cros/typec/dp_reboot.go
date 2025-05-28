// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/remote/typec/typecswitch"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DpReboot,
		Desc:     "Check that a DisplayPort display enumerates successfully after reboot",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		SoftwareDeps: []string{"reboot"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort", "typec.McciPath", "typec.UnigrafUri"},
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_dp_bringup", "typec_unigraf274"},
			Val:       5,
			Timeout:   7 * time.Minute,
		}, {
			Name:    "stress",
			Val:     25,
			Timeout: 35 * time.Minute,
		}},
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
//	 ____network___
//	|              |
//	|              |
//	Host -------- DUT ----- MCCI (`portUsed`) ---- DP display (can be connected via DP Type-C dock).
//	|                              |
//	|______________________________|
func DpReboot(ctx context.Context, s *testing.State) {
	numIterations := s.Param().(int)
	d := s.DUT()

	s.Log("Number of iterations: ", numIterations)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	sw, err := typecswitch.GetSwitch(ctx, s)
	if err != nil {
		s.Fatal("Failed to get MCCI switch handle: ", err)
	}
	defer sw.Close(cleanupCtx)

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performDpRebootIteration(ctx, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performDpRebootIteration runs 1 iteration of the DisplayPort reboot test.
func performDpRebootIteration(ctx context.Context, d *dut.DUT, sw typecswitch.Switch) error {
	// Disconnect the dock/display.
	if err := sw.DisablePorts(ctx); err != nil {
		return errors.Wrap(err, "failed to switch off the port")
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
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to switch on the port")
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
