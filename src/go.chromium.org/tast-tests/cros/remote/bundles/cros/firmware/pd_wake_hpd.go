// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PDWakeHPD,
		Desc: "Tests if the DUT wakes from docked mode",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"jasonyuan@google.com",     // Test author
		},
		BugComponent: "b:298675713", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.MKBPEvent()),
		Timeout:      60 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}},
	})
}

type hpdConfigs struct {
	hpdBeforeSuspend servo.HPDLevelValue
	hpdAfterSuspend  servo.HPDLevelValue
	hpdIRQSuspend    bool
	hpdBeforeResume  servo.HPDLevelValue
	expectedWake     bool
}

func PDWakeHPD(ctx context.Context, s *testing.State) {
	TestFailures := 0

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	// TODO(b/298675713): change hwdep to depend on pdc, not just brox
	board, err := h.Reporter.Board(ctx)
	if err != nil {
		s.Fatal("Failed to get board name: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	input := servo.TypeCInfo{DPMode: servo.DPEnable, PinsCDEF: "CD"}
	if err := h.Servo.ServoSetDPConfigs(ctx, &input, servo.MFPrefEnable); err != nil {
		s.Fatal("Failed to set DP alt-mode: ", err)
	}

	testing.ContextLog(ctx, "verifying DP is enabled")
	typecInfo, err := h.Servo.GetTypeCInfo(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed to retrieve type-c information: ", err)
	}
	if typecInfo.DPMode != servo.DPEnable {
		s.Fatal("Type-c DP did not enable")
	}

	configs := []hpdConfigs{
		// The elements in order are:
		// hpdBeforeSuspend, hpdAfterSuspend, hpdIRQSuspend, hpdBeforeResume, expectedWake
		hpdConfigs{servo.HPDHigh, servo.HPDHigh, false, servo.HPDHigh, false}, // always hpd high
		hpdConfigs{servo.HPDHigh, servo.HPDHigh, false, servo.HPDLow, false},  // hpd low in suspend
		hpdConfigs{servo.HPDHigh, servo.HPDLow, false, servo.HPDHigh, true},   // hpd low and then high in suspend
		hpdConfigs{servo.HPDHigh, servo.HPDHigh, true, servo.HPDHigh, false},  // hpd irq in suspend
		hpdConfigs{servo.HPDLow, servo.HPDLow, false, servo.HPDHigh, true},    // suspend then hpd high
	}

	for idx, hpd := range configs {
		testing.ContextLogf(ctx, "before suspend, setting hpd to %s", string(hpd.hpdBeforeSuspend))
		if err := h.Servo.SetHPD(ctx, hpd.hpdBeforeSuspend); err != nil {
			s.Fatal("Failed to set hpd: ", err)
		}

		cmd := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--delay=3")
		if err := cmd.Start(); err != nil {
			s.Fatal("Failed to invoke powerd_dbus_suspend: ", err)
		}

		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S3", "S0ix"); err != nil {
			s.Fatal("Failed to suspend: ", err)
		}

		irqString := "no interrupt"
		if hpd.hpdIRQSuspend {
			irqString = "interrupt"
		}
		testing.ContextLogf(ctx, "after suspend, setting hpd to %s, %s, %s", string(hpd.hpdAfterSuspend), irqString, string(hpd.hpdBeforeResume))
		if err := h.Servo.SetHPD(ctx, hpd.hpdAfterSuspend); err != nil {
			s.Fatal("Failed to set hpd: ", err)
		}
		// GoBigSleepLint: make sure we're not spamming the hpd pin
		testing.Sleep(ctx, 1*time.Second)
		if hpd.hpdIRQSuspend {
			if err := h.Servo.SetHPD(ctx, servo.HPDirq); err != nil {
				s.Fatal("Failed to set hpd: ", err)
			}
		}
		// GoBigSleepLint: make sure we're not spamming the hpd pin
		testing.Sleep(ctx, 1*time.Second)
		if err := h.Servo.SetHPD(ctx, hpd.hpdBeforeResume); err != nil {
			s.Fatal("Failed to set hpd: ", err)
		}

		// GoBigSleepLint: give a bit of time for the DUT to respond to the hpd pin
		testing.Sleep(ctx, 1*time.Second)
		powerState, getPowerStateErr := h.Servo.GetECSystemPowerState(ctx)
		if getPowerStateErr != nil {
			s.Fatal("Failed to retrieve the power state of the DUT ", getPowerStateErr)
		}

		// ec policy always wake on high regardless of previous state
		if hpd.expectedWake || board != "brox" {
			if powerState != "S0" {
				s.Errorf("Test case %d: Expected power state: S0, actual: %s", idx, string(powerState))
				TestFailures++
			}
		} else {
			if powerState != "S3" && powerState != "S0ix" {
				s.Errorf("Test case %d: Expected power state: S3 or S0ix, actual: %s", idx, string(powerState))
				TestFailures++
			}
		}

		if powerState != "S0" {
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
				testing.ContextLog(ctx, "Failed to power on DUT: ", err)
			}
			if err := h.WaitConnect(ctx); err != nil {
				s.Fatal("Failed to boot after test: ", err)
			}
		}
	}

	if TestFailures > 0 {
		s.Fatalf("Failed %d cases", TestFailures)
	}
}
