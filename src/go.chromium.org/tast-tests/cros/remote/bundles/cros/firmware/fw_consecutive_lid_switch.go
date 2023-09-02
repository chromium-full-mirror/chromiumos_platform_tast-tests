// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var (
	runCycles = testing.RegisterVarString(
		"firmware.FWConsecutiveLidSwitch.RunCycles",
		"100",
		"The number of lid open/close cycles")
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FWConsecutiveLidSwitch, LacrosStatus: testing.LacrosVariantUnneeded, Desc: "Trigger lid switch on and off many times consecutively",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		Fixture:      fixture.NormalMode,
		// This test could take a long time, at least 1 minute per retry
		Timeout: 40 * time.Minute,
	})
}

// FWConsecutiveLidSwitch triggers lid state on and off via Servo
// after logging to Chrome as Guest and then checks if boot ID is
// the same as before
func FWConsecutiveLidSwitch(ctx context.Context, s *testing.State) {

	const (
		sleepDelay time.Duration = 1 * time.Second
		wakeDelay  time.Duration = 1 * time.Second
	)

	cycles, err := strconv.Atoi(runCycles.Value())
	if err != nil {
		s.Fatalf("Bad value %v is set for variable %v: %v",
			runCycles.Value(), runCycles.Name(), err)
	}
	if cycles < 1 {
		s.Fatalf("%v must be positive instead of %v",
			runCycles.Name(), cycles)
		return
	}

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	if err := h.RequireRPCUtils(ctx); err != nil {
		s.Fatal("Failed to require RPC utils: ", err)
	}

	// Need to login, because on the login screen the DUT goes off with
	// the lid closed, whereas the intent is to exercise suspend.
	s.Log("Creating new Chrome instance")
	if _, err := h.RPCUtils.NewChrome(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to create Chrome instance: ", err)
	}

	s.Log("Opening lid for the first time to be sure")
	if err := h.Servo.OpenLid(ctx); err != nil {
		s.Fatal("Failed to open lid: ", err)
	}

	initialBootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		s.Fatal("Failed to acquire current boot ID: ", err)
	}
	s.Log("Current boot ID: ", initialBootID)

	for r := 0; r < cycles; r++ {
		s.Logf("Consecutive lid switch %d/%d", r, cycles)

		if err := h.Servo.CloseLid(ctx); err != nil {
			s.Fatal("Failed to close lid: ", err)
		}

		h.CloseRPCConnection(ctx)

		// GoBigSleepLint: DuT operation dependency
		if err := testing.Sleep(ctx, sleepDelay); err != nil {
			s.Fatal("Failed to sleep during closed lid delay: ", err)
		}

		if err := h.DUT.WaitUnreachable(ctx); err != nil {
			s.Fatal("Failed to make DUT unreachable: ", err)
		}

		if err := h.Servo.OpenLid(ctx); err != nil {
			s.Fatal("Failed to open lid: ", err)
		}

		if err := h.DUT.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to connect to DUT: ", err)
		}

		afterLidBootID, err := h.Reporter.BootID(ctx)
		if err != nil {
			s.Fatal("Failed to acquire boot ID after lid switch: ", err)
		}
		if afterLidBootID != initialBootID {
			s.Fatalf("Initial boot ID does not match boot ID after lid switch: got %s; want %s", afterLidBootID, initialBootID)
		}
	}
}
