// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	input "go.chromium.org/tast-tests/cros/remote/inputs"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"

	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TouchColdboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Touchscreen: Cold boot (S0-S5) with operation for 10 cycles",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.inputs.TouchscreenService"},
		HardwareDeps: hwdep.D(hwdep.TouchScreen(), hwdep.X86()),
		Attr:         []string{"group:intel-nda"},
		BugComponent: "b:157291", // ChromeOS > External > Intel.
		Fixture:      fixture.NormalMode,
		Timeout:      15 * time.Minute,
	})
}

func TouchColdboot(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	dut := s.DUT()

	performEVTest := func() {
		// Connect to the gRPC service on the DUT.
		cl, err := rpc.Dial(ctx, dut, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}

		// Declare a rpc service for detecting touchscreen.
		touchscreen := inputs.NewTouchscreenServiceClient(cl.Conn)

		// Start a logged-in Chrome session, which is required prior to TouchscreenTap
		if _, err := touchscreen.NewChrome(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to start a new Chrome for the touchscreen service: ", err)
		}
		defer touchscreen.CloseChrome(ctx, &empty.Empty{})

		devPath, err := touchscreen.FindPhysicalTouchscreen(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("Failed to get touchscreen device path: ", err)
		}

		cmd, scannTouchscreen, err := input.DeviceScanner(ctx, h, devPath.Path)
		if err != nil {
			s.Fatal("Failed to get touchscreen scanner: ", err)
		}
		defer cmd.Wait()
		defer cmd.Abort()

		// Emulate the action of tapping on a touch screen.
		if _, err := touchscreen.TouchscreenTap(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to perform a tap on the touch screen: ", err)
		}

		if err := input.EvtestMonitor(ctx, scannTouchscreen); err != nil {
			s.Fatal("Failed to monitor evtest for touchscreen: ", err)
		}
	}

	iterations := 10
	for i := 1; i <= iterations; i++ {
		s.Logf("Iteration: %d/%d", i, iterations)

		performEVTest()

		powerState := "S5"
		if err := powercontrol.ShutdownAndWaitForPowerState(ctx, h.ServoProxy, dut, powerState); err != nil {
			s.Fatalf("Failed to shutdown and wait for %q powerstate: %v", powerState, err)
		}

		if err := powercontrol.PowerOntoDUT(ctx, h.ServoProxy, dut); err != nil {
			s.Fatal("Failed to wake up DUT: ", err)
		}

		performEVTest()

		// Performing prev_sleep_state check.
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, 5); err != nil {
			s.Fatal("Failed to validate previous sleep state: ", err)
		}
	}
}
