// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DevModeECRW,
		Desc: "Tests that DUT cannot boot to dev mode from rec mode while in EC RW",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.NormalMode,
		Timeout:      10 * time.Minute,
	})
}

func DevModeECRW(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to require config: ", err)
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()
	restore, err := utils.EnableSoftwareSync(ctx, h, true)
	if err != nil {
		if restore != nil {
			s.Log("Failed to clear disable software sync flag: ", err)
			restore(cleanupContext, s)
		} else {
			s.Fatal("Failed to clear disable software sync flag: ", err)
		}
	}
	defer restore(cleanupContext, s)

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	// Reset DUT at end of test because its potentially stuck in state where it can't connect.
	defer func(ctx context.Context) {
		s.Log("Set power state to reset with servo")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to power off DUT with servo: ", err)
		}
	}(cleanupContext)

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	s.Log("Set power state to off with servo")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
		s.Fatal("Failed to power off DUT with servo: ", err)
	}

	s.Log("Checking ec_active_copy is RW or RW_B")
	activeCopy, err := h.Servo.GetString(ctx, "ec_active_copy")
	if err != nil {
		s.Fatal("EC active copy failed: ", err)
	}
	if !strings.HasPrefix(activeCopy, "RW") {
		s.Fatalf("EC active copy incorrect, got %q want RW", activeCopy)
	}
	s.Log("Current EC active copy: ", activeCopy)

	s.Log("Use keyboard recovery hostevent to move to rec mode")
	if err := h.Servo.SetHostevent(ctx, servo.HosteventKeyboardRecovery); err != nil {
		s.Fatal("Failed to set keyboard recovery hostevent: ", err)
	}

	s.Log("Pressing power key to power on")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
		s.Fatal("Failed to press power key on DUT: ", err)
	}

	s.Log("Pressing Ctrl-D or equivalent to get to Dev mode (expected to fail)")
	if err := ms.RecScreenToDevMode(ctx, firmware.SkipWaitConnect); err != nil {
		testing.ContextLog(ctx, "Failed to transition to dev mode from rec mode, this is expected behavior")
	} else {
		s.Fatal("Expected transitioning to dev mode from recovery screen while in EC_RW to fail, but succeeded instead")
	}

	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to perform a warm reset: ", err)
	}
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after warm reset: ", err)
	}

	r := reporters.New(h.DUT)
	devSWBoot, err := r.CrossystemParam(ctx, reporters.CrossystemParamDevswBoot)
	if err != nil {
		s.Error("Failed to get crossystem devsw_boot value: ", err)
	}
	mainFWType, err := r.CrossystemParam(ctx, reporters.CrossystemParamMainfwType)
	if err != nil {
		s.Error("Failed to get crossystem mainfw_type value: ", err)
	}

	currPowerState, err := h.Servo.GetECSystemPowerState(ctx)
	if err != nil {
		s.Error("Failed to get current power state: ", err)
	}

	if devSWBoot == "0" && mainFWType == "normal" {
		s.Log("DUT failed to boot into dev mode which is expected, booted to normal mode instead")
	} else if currPowerState == "G3" || currPowerState == "S5" {
		s.Log("DUT failed to boot into dev mode which is expected, powered off instead")
	} else {
		activeCopy, err = h.Servo.GetString(ctx, "ec_active_copy")
		if err != nil {
			s.Error("Failed to get EC active copy: ", err)
		}
		s.Fatalf("Unexpected outcome, device ended with EC in %q, powerstate %q, devsw_boot: %v, mainfw_type: %v", activeCopy, currPowerState, devSWBoot, mainFWType)
	}

}
