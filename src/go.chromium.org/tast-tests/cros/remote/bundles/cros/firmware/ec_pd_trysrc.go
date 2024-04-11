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
		Func:         ECPDTrysrc,
		Desc:         "Verify USB-C/PD TrySrc",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"asemjonovs@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		Params: firmware.AddPDPorts([]testing.Param{{
			Name: "normal",
			Val:  firmware.PDTestParams{},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC: firmware.CCPolarityFlipped,
			},
		}}, []string{"group:firmware", "firmware_pd_unstable"}),
	})
}

const (
	pdDisconnectTime    int           = 1
	pdConnectTime       time.Duration = 4 * time.Second
	pdConnectIterations int           = 20
	pdStableDelayTime   time.Duration = 3 * time.Second

	pdTrySrcOffThreshold float32 = 15.0
	pdTrySrcOnThreshold  float32 = 96.0
)

func executeConnectSequence(ctx context.Context, s *testing.State, trySrcSupported bool) (int, int) {
	h := s.FixtValue().(*fixture.Value).Helper
	snkStats := 0
	srcStats := 0
	srcConnect := []string{"SRC_READY"}
	snkConnect := []string{"SNK_READY"}

	for i := 0; i < pdConnectIterations; i++ {
		if trySrcSupported {
			h.Servo.SetPDTrySrc(ctx, 1)
		} else {
			h.Servo.SetPDTrySrc(ctx, 0)
		}
		if state, err := h.Servo.ServoGetConnectedStateAfterCCReconnect(ctx, pdDisconnectTime); err != nil {
			s.Fatal("Failed to get connected state after reconnect: ", err)
		} else {
			if state == snkConnect[0] {
				snkStats++
				testing.ContextLog(ctx, "Power Role = SNK")
			} else if state == srcConnect[0] {
				srcStats++
				testing.ContextLog(ctx, "Power Role = SRC")
			}
			// GoBigSleepLint: Wait a bit before the next iteration, in case any PR_Swap
			if err := testing.Sleep(ctx, pdStableDelayTime); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
		}
	}
	testing.ContextLogf(ctx, "SNK = %d: SRC = %d: Total = %d",
		snkStats, srcStats, pdConnectIterations)
	return snkStats, srcStats
}

func ECPDTrysrc(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if err := h.Servo.SetDUTDualRole(ctx, servo.USBPdDualRoleOn); err != nil {
		s.Fatal("Could not enable DRP on EC")
	}

	if err := h.Servo.ServoSetDUTDualRole(ctx, servo.USBPdDualRoleOn); err != nil {
		s.Fatal("Could not enable DRP on Servo")
	}

	// GoBigSleepLint: Setting DRP on ServoV4 ('usbc_action drp') triggers reconnect
	// Wait some time to ensure that no operation will occur during test
	if err := testing.Sleep(ctx, pdConnectTime); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if trySrcSupported, err := h.Servo.SetPDTrySrc(ctx, 1); err != nil {
		s.Fatal("DUT does not support Try.SRC feature: ", err)
	} else {
		if trySrcSupported {
			// Run disconnect/connect sequence with Try.SRC enabled
			snkOn, srcOn := executeConnectSequence(ctx, s, true)
			totalOn := snkOn + srcOn
			trySrcOn := float32(snkOn) * 100.0 / float32(totalOn)
			testing.ContextLogf(ctx, "SNK ratio with Try.SRC enabled = %f", trySrcOn)

			if trySrcOn < pdTrySrcOnThreshold {
				s.Fatalf("SRC %% = %.1f: Must be >  %.1f", trySrcOn, pdTrySrcOnThreshold)
			}
		}

		// Run disconnect/connect sequence with Try.SRC disabled
		snkOff, srcOff := executeConnectSequence(ctx, s, false)
		totalOff := snkOff + srcOff
		trySrcOff := float32(snkOff) * 100.0 / float32(totalOff)

		// When Try.SRC is off, ideally the SNK/SRC ratio will be close to
		// 50%. However, in practice there is a wide range related to the
		// dualrole swap timers in firmware.
		if trySrcOff < pdTrySrcOffThreshold || trySrcOff > 100-pdTrySrcOffThreshold {
			s.Fatalf("SRC %% = %.1f: Must be > %.1f & < %.1f", trySrcOff,
				pdTrySrcOffThreshold, 100-pdTrySrcOffThreshold)
		}
	}
}
