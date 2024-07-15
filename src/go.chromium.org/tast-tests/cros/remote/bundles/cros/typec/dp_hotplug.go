// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DpHotplug,
		Desc:     "Check that a DisplayPort Alternate Mode display enumerates successfully on hotplug",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort", "typec.McciPath"},
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_dp_bringup"},
			Val:       10,
			Timeout:   7 * time.Minute,
		}, {
			Name:    "stress",
			Val:     50,
			Timeout: 35 * time.Minute,
		}},
	})
}

// DpHotplug does the following:
//
// - Disconnect the DP display via MCCI switch.
// - Verify that there is no DP display present on the system.
// - Reconnect the DP display via MCCI switch.
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
func DpHotplug(ctx context.Context, s *testing.State) {
	numIterations := s.Param().(int)
	d := s.DUT()

	s.Log("Number of iterations: ", numIterations)

	portUsed, err := strconv.Atoi(s.RequiredVar("typec.McciPort"))
	if err != nil {
		s.Fatal("Failed to parse MCCI port commandline variable: ", err)
	}

	path, _ := s.Var("typec.McciPath")
	sw, err := mcci.GetSwitch(s.RequiredVar("typec.McciSerial"), path)
	if err != nil {
		s.Fatal("Failed to get MCCI switch handle: ", err)
	}
	defer sw.Close()

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performDpHotplugIteration(ctx, d, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performDpHotplugIteration runs 1 iteration of the DP hotplug test.
func performDpHotplugIteration(ctx context.Context, d *dut.DUT, sw *mcci.Switch, mcciPort int) error {
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
		return errors.Wrap(err, "failed DP presence check")
	}

	return nil
}
