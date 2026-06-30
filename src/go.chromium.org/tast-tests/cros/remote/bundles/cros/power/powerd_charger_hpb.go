// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PowerdChargerHpb,
		Desc: "Verify Hybrid Power Boost charger behavior in powerd",
		Contacts: []string{
			"chromeos-power-team@google.com", // CrOS power team
			"mqg@google.com",                 // test author
		},
		BugComponent: "b:1361410",
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr: []string{
			"group:power",
			"power_weekly_misc",
		},
		HardwareDeps: hwdep.D(
			hwdep.ChromeEC(),
			hwdep.Battery(),
			hwdep.ECBuildConfigOptions("PLATFORM_EC_CHARGER_HYBRID_POWER_BOOST"),
		),
		Fixture: fixture.NormalMode,
	})
}

const (
	// hpbSettleTime is the duration to wait for the power delivery state to settle.
	hpbSettleTime = 2 * time.Second
)

// verifyLinePowerEnumType runs power_supply_info, parses it, and verifies the
// line power enum type. It calls s.Fatal if any step fails or the type does not
// match the expected value.
func verifyLinePowerEnumType(ctx context.Context, s *testing.State, want string) {
	h := s.FixtValue().(*fixture.Value).Helper
	out, err := h.DUT.Conn().CommandContext(ctx, "power_supply_info").Output()
	if err != nil {
		s.Fatal("Failed to run power_supply_info: ", err)
	}

	supplyInfo, err := power.ParseSupplyInfo(string(out))
	if err != nil {
		s.Fatal("Failed to parse power_supply_info: ", err)
	}

	got, ok := supplyInfo.LinePower["enum type"]
	if !ok {
		s.Fatal("`enum type` key not found in Line Power info")
	}

	if got != want {
		s.Fatalf("Unexpected Line Power enum type: got %q, want %q", got, want)
	}
	s.Logf("Successfully verified Line Power enum type is %q", want)
}

func PowerdChargerHpb(ctx context.Context, s *testing.State) {
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
		// Restore servo to default charging.
		h.Servo.SetString(ctx, "usbc_pr", "20")
		h.Servo.SetString(ctx, "servo_dts_mode", "on")
		h.Servo.SetString(ctx, "servo_pd_role", "src")

		if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
			s.Fatal("Failed to set charger status to connected: ", err)
		}

		verifyLinePowerEnumType(ctx, s, "AC")
	}()

	verifyLinePowerEnumType(ctx, s, "AC")

	// Configure insufficient adapter, 5 volts.
	h.Servo.SetString(ctx, "usbc_pr", "5")

	// GoBigSleepLint: Allow some time for PD state to settle.
	if err := testing.Sleep(ctx, hpbSettleTime); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	verifyLinePowerEnumType(ctx, s, "Low voltage non-charging")
}
