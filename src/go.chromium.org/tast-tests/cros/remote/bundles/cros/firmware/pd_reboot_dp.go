// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PDRebootDP,
		Desc: "Tests if the DUT connectes in DP alt-mode for usb-c peripherals during shutdown and boot",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"jasonyuan@google.com",     // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.TypecStatus()),
		Timeout:      60 * time.Minute,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Name: "plug",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOff,
				DPAltPlug: true,
			},
		}, {
			Name: "plug_snk",
			Val: firmware.PDTestParams{
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOff,
				DPAltPlug: true,
			},
		}, {
			Name: "receptacle",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOff,
				DPAltPlug: false,
			},
		}, {
			Name: "receptacle_snk",
			Val: firmware.PDTestParams{
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOff,
				DPAltPlug: false,
			},
		}},
	})
}

type plugHpdConfigs struct {
	hpdBeforeSuspend servo.HPDLevelValue
	hpdAfterSuspend  servo.HPDLevelValue
	hpdBeforeResume  servo.HPDLevelValue
}

func PDRebootDP(ctx context.Context, s *testing.State) {
	TestFailures := 0

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	input := servo.TypeCInfo{DPMode: servo.DPEnable, PinsCDEF: "CD"}
	if err := h.Servo.ServoSetDPConfigs(ctx, &input, servo.MFPrefEnable); err != nil {
		s.Fatal("Failed to set DP alt-mode: ", err)
	}
	if err := h.Servo.SetPlug(ctx, testParams.DPAltPlug); err != nil {
		s.Fatal("Failed to set plug: ", err)
	}

	testing.ContextLog(ctx, "verifying DP is enabled")
	typecInfo, err := h.Servo.GetTypeCInfo(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed to retrieve type-c information: ", err)
	}
	if typecInfo.DPMode != servo.DPEnable {
		s.Fatal("Type-c DP did not enable")
	}

	configs := []plugHpdConfigs{
		// The elements in order are:
		// hpdBeforeSuspend, hpdAfterSuspend, hpdBeforeResume
		plugHpdConfigs{servo.HPDHigh, servo.HPDHigh, servo.HPDHigh}, // always plugged
		plugHpdConfigs{servo.HPDHigh, servo.HPDHigh, servo.HPDLow},  // unplug in shutdown
		plugHpdConfigs{servo.HPDHigh, servo.HPDLow, servo.HPDHigh},  // replug in shutdown
		plugHpdConfigs{servo.HPDLow, servo.HPDLow, servo.HPDHigh},   // shutdown and plug
		plugHpdConfigs{servo.HPDLow, servo.HPDHigh, servo.HPDLow},   // shutdown and plug and unplug
	}

	for idx, hpd := range configs {
		testing.ContextLogf(ctx, "before suspend, setting plug to %s", hpd.hpdBeforeSuspend)
		if err := h.Servo.SetHPD(ctx, hpd.hpdBeforeSuspend); err != nil {
			s.Fatal("Failed to set hpd: ", err)
		}

		if err := firmware.ShutdownDUT(ctx, h); err != nil {
			s.Fatal("Failed to shutdown: ", err)
		}

		testing.ContextLogf(ctx, "after suspend, setting plug to %s, %s", hpd.hpdAfterSuspend, hpd.hpdBeforeResume)
		if err := h.Servo.SetHPD(ctx, hpd.hpdAfterSuspend); err != nil {
			s.Fatal("Failed to set hpd: ", err)
		}
		// GoBigSleepLint: make sure we're not spamming the hpd pin
		testing.Sleep(ctx, 1*time.Second)
		if err := h.Servo.SetHPD(ctx, hpd.hpdBeforeResume); err != nil {
			s.Fatal("Failed to set hpd: ", err)
		}

		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx, firmware.SkipPDRoleSnk); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}

		testing.ContextLog(ctx, "verifying DP is still enabled")
		typecInfo, err := h.Servo.GetTypeCInfo(ctx, h.DUT)
		if err != nil {
			s.Fatal("Failed to retrieve type-c information: ", err)
		}
		if typecInfo.DPMode != servo.DPEnable {
			s.Errorf("Test case %d: Type-c DP did not enable", idx)
			TestFailures++
		}
	}

	if TestFailures > 0 {
		s.Fatalf("Failed %d cases", TestFailures)
	}
}
