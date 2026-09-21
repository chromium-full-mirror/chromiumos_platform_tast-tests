// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECPDTrysrc,
		Desc: "Verify USB-C/PD TrySrc",
		Contacts: []string{
			"chromeos-faft@google.com",
			"asemjonovs@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw", "firmware_bios_pdc"},
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}},
	})
}

const (
	pdDisconnectTime    time.Duration = 2 * time.Second
	pdSetupPollTimeout  time.Duration = 20 * time.Second
	pdSetupPollInterval time.Duration = 4 * time.Second
	pdConnectIterations int           = 20
	pdStableDelayTime   time.Duration = 3 * time.Second

	pdTrySrcOffThreshold float32 = 15.0
	pdTrySrcOnThreshold  float32 = 96.0
)

func executeConnectSequence(ctx context.Context, s *testing.State, trySrcSupported bool) (int, int) {
	h := s.FixtValue().(*fixture.Value).Helper
	snkStats := 0
	srcStats := 0
	srcConnect := "SRC_READY"
	snkConnect := "SNK_READY"
	var trySrcInt int

	trySrcInt = 0
	if trySrcSupported {
		trySrcInt = 1
	}

	// servo_uart_timeout to a larger value than normal
	if err := h.Servo.SetFloat(ctx, servo.UARTCmdTimeout, 10); err != nil {
		s.Fatal("Failed to set servo uart timeout: ", err)
	}
	defer func() {
		if err := h.Servo.SetFloat(ctx, servo.UARTCmdTimeout, servo.DefaultUARTCmdTimeout); err != nil {
			s.Fatal("Failed to reset servo uart timeout: ", err)
		}
	}()

	for i := 0; i < pdConnectIterations; i++ {
		if _, err := h.Servo.SetPDTrySrc(ctx, servo.PDPortUnderTest, trySrcInt); err != nil {
			testing.ContextLogf(ctx, "Failed Enabling TrySrc: %q", err)
		}
		// Disconnect time from 2 to 2.5 seconds
		randDisconnectTime := pdDisconnectTime + time.Duration(rand.Float32()*float32(time.Second)/2)
		testing.ContextLogf(ctx, "Disconnect time = %s", randDisconnectTime)
		if state, err := h.Servo.ServoGetConnectedStateAfterCCReconnect(ctx, randDisconnectTime); err != nil {
			testing.ContextLogf(ctx, "Failed to get connected state after reconnect: %q", err)
		} else {
			if state == snkConnect {
				snkStats++
				testing.ContextLog(ctx, "Servo Power Role = SNK")
			} else if state == srcConnect {
				srcStats++
				testing.ContextLog(ctx, "Servo Power Role = SRC")
			}
		}
		// GoBigSleepLint: Wait a bit before the next iteration, in case any PR_Swap
		if err := testing.Sleep(ctx, pdStableDelayTime); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}
	testing.ContextLogf(ctx, "Servo SNK = %d: SRC = %d: Total = %d",
		snkStats, srcStats, pdConnectIterations)
	return snkStats, srcStats
}

// waitForStableConnection polls until the Servo is in a connected PD state, for up to 10 sec.
func waitForStableConnection(ctx context.Context, s *testing.State) error {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdState, err := h.Servo.GetServoPDState(ctx)
		if err != nil {
			return errors.Wrap(err, "cannot get Servo PD state")
		}

		if !pdState.IsPDReady() {
			return errors.Errorf("Servo is not yet PD ready (%s)", pdState.PEStateName)
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 2 * time.Second}); err != nil {
		return errors.Wrap(err, "poll timed out")
	}

	return nil
}

func ECPDTrysrc(ctx context.Context, s *testing.State) {
	var trySrcSupported bool

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if err := h.Servo.EnableServoConsoleChannel(ctx, "usbpd"); err != nil {
		s.Fatal("Failed to enable usbpd console channel: ", err)
	}

	if err := h.Servo.SetDualroleState(ctx, servo.DROn); err != nil {
		s.Fatal("Could not enable DRP on EC: ", err)
	}

	if err := h.Servo.ServoSetDualRole(ctx, servo.USBPdDualRoleOn); err != nil {
		s.Fatal("Could not enable DRP on Servo: ", err)
	}

	if err := waitForStableConnection(ctx, s); err != nil {
		s.Fatal("PD connection not established after configuring Servo: ", err)
	}

	// GoBigSleepLint: Setting DRP on ServoV4 ('usbc_action drp') triggers reconnect
	// poll SetPDTrySrc to make sure servo finished connecting to DUT
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		trySrcTmp, err := h.Servo.SetPDTrySrc(ctx, servo.PDPortUnderTest, 1)
		if err != nil {
			return errors.Wrap(err, "failed to set up Try.SRC")
		}

		trySrcSupported = trySrcTmp
		return nil
	}, &testing.PollOptions{Timeout: pdSetupPollTimeout, Interval: pdSetupPollInterval}); err != nil {
		s.Fatal("DUT does not support Try.SRC feature: ", err)
	}

	// Ensure we have a stable connection before starting the experiment
	if err := waitForStableConnection(ctx, s); err != nil {
		s.Fatal("PD connection not established after probing TrySRC: ", err)
	}

	if trySrcSupported {
		// Run disconnect/connect sequence with Try.SRC enabled
		snkOn, srcOn := executeConnectSequence(ctx, s, true)
		totalOn := float32(snkOn + srcOn)
		trySrcOn := float32(snkOn) * 100.0 / totalOn
		testing.ContextLogf(ctx, "Servo SNK ratio with DUT Try.SRC enabled = %f", trySrcOn)

		if trySrcOn < pdTrySrcOnThreshold {
			s.Fatalf("DUT SRC %% = %.1f: Must be >  %.1f", trySrcOn, pdTrySrcOnThreshold)
		}
	}

	// Run disconnect/connect sequence with Try.SRC disabled
	snkOff, srcOff := executeConnectSequence(ctx, s, false)
	totalOff := float32(snkOff + srcOff)
	trySrcOff := float32(snkOff) * 100.0 / totalOff
	testing.ContextLogf(ctx, "Servo SNK ratio with DUT Try.SRC disabled = %f", trySrcOff)

	// When Try.SRC is off, ideally the SNK/SRC ratio will be close to
	// 50%. However, in practice there is a wide range related to the
	// dualrole swap timers in firmware.
	if trySrcOff < pdTrySrcOffThreshold || trySrcOff > 100-pdTrySrcOffThreshold {
		s.Fatalf("DUT SRC %% = %.1f: Must be > %.1f & < %.1f", trySrcOff,
			pdTrySrcOffThreshold, 100-pdTrySrcOffThreshold)
	}
}
