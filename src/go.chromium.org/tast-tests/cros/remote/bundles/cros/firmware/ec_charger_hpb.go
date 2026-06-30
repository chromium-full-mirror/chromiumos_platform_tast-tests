// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECChargerHpb,
		Desc: "Verify Hybrid Power Boost charger",
		Contacts: []string{
			"chromeos-faft@google.com",
			"asemjonovs@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery(), hwdep.ECBuildConfigOptions("PLATFORM_EC_CHARGER_HYBRID_POWER_BOOST")),
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
	})
}

const (
	hpbSettleTime time.Duration = 500 * time.Millisecond
)

func ECChargerHpb(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove watchdog for ccd: ", err)
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to RPC: ", err)
	}

	defer func() {
		s.Log("Reconnect to charger on test end")
		// Restore servo to default charging
		h.Servo.SetString(ctx, "usbc_pr", "20")
		h.Servo.SetString(ctx, "servo_dts_mode", "on")
		h.Servo.SetString(ctx, "servo_pd_role", "src")

		if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
			s.Fatal("Failed to set charger status to connected: ", err)
		}
	}()
	ectool := firmware.NewECTool(h.DUT, firmware.ECToolNameMain)

	// By default, adapter should be sufficient
	sufficient, err := ectool.IsAdapterSufficient(ctx)
	if err != nil {
		s.Fatal("Failed to get IsAdapterSufficient: ", err)
	}
	if sufficient == 0 {
		s.Fatal("Expected sufficient adapter")
	}

	// Configure insufficient adapter, 5 volts
	h.Servo.SetString(ctx, "usbc_pr", "5")

	// GoBigSleepLint: Allow some time for PD state to settle
	if err := testing.Sleep(ctx, hpbSettleTime); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Verify insufficient adapter
	sufficient, err = ectool.IsAdapterSufficient(ctx)
	if err != nil {
		s.Fatal("Failed to get IsAdapterSufficient: ", err)
	}
	if sufficient != 0 {
		s.Fatal("Expected insufficient adapter")
	}
}
