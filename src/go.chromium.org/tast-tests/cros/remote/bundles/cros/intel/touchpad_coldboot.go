// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	input "go.chromium.org/tast-tests/cros/remote/inputs"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TouchpadColdboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Touchpad: Cold boot (S0-S5) with operation for 10 cycles",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.inputs.TouchpadService"},
		Attr:         []string{"group:intel-nda"},
		HardwareDeps: hwdep.D(hwdep.Touchpad(), hwdep.X86()),
		Fixture:      fixture.NormalMode,
		Timeout:      15 * time.Minute,
	})
}

func TouchpadColdboot(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	h := s.FixtValue().(*fixture.Value).Helper
	rpcHint := s.RPCHint()

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	dut := s.DUT()

	// Due to any reason if DUT is not up at the end of test, make the DUT up.
	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, h.ServoProxy, dut); err != nil {
				s.Fatal("Failed to power on DUT at cleanup: ", err)
			}
		}
	}(cleanupCtx)

	iterations := 10
	for i := 1; i <= iterations; i++ {
		s.Logf("Iteration: %d/%d", i, iterations)

		if err := performEVTest(ctx, dut, h, rpcHint); err != nil {
			s.Fatal("Failed to perform EV test: ", err)
		}

		powerState := "S5"
		if err := powercontrol.ShutdownAndWaitForPowerState(ctx, h.ServoProxy, dut, powerState); err != nil {
			s.Fatalf("Failed to shutdown and wait for %q powerstate: %v", powerState, err)
		}

		if err := powercontrol.PowerOntoDUT(ctx, h.ServoProxy, dut); err != nil {
			s.Fatal("Failed to wake up DUT: ", err)
		}

		if err := performEVTest(ctx, dut, h, rpcHint); err != nil {
			s.Fatal("Failed to perform EV test: ", err)
		}

		if err := powercontrol.ValidatePrevSleepState(ctx, dut, 5); err != nil {
			s.Fatal("Failed to validate previous sleep state: ", err)
		}
	}
}

func performEVTest(ctx context.Context, dut *dut.DUT, h *firmware.Helper, rpcHint *testing.RPCHint) error {

	// Connect to the gRPC service on the DUT.
	cl, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	// Declare a grpc service for detecting touchpad.
	touchpad := inputs.NewTouchpadServiceClient(cl.Conn)

	// Start a logged-in Chrome session, which is required prior to TouchpadSwipe.
	if _, err := touchpad.NewChrome(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start a new Chrome for the touchpad service ")
	}
	defer touchpad.CloseChrome(ctx, &empty.Empty{})

	devPath, err := touchpad.FindPhysicalTouchpad(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get touchpad device path ")
	}

	cmd, scannTouchpad, err := input.DeviceScanner(ctx, h, devPath.Path)
	if err != nil {
		return errors.Wrap(err, "failed to get touchpad scanner")
	}
	defer cmd.Wait()
	defer cmd.Abort()

	// Emulate swiping on a touch pad.
	if _, err := touchpad.TouchpadSwipe(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to perform a tap on the touch pad")
	}

	if err := input.EvtestMonitor(ctx, scannTouchpad); err != nil {
		return errors.Wrap(err, "failed to monitor evtest for touchpad ")
	}
	return nil
}
