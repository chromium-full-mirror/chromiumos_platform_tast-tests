// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type batteryStatusTestParam struct {
	iter       int
	tabletMode bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         BatteryStatusOnACRemoval,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Battery status and stop charging upon removal of AC",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.power.BatteryService", "tast.cros.firmware.UtilsService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Fixture:      fixture.NormalMode,
		Params: []testing.Param{{
			Name: "clamshell",
			Val: batteryStatusTestParam{
				iter:       5,
				tabletMode: false,
			},
			ExtraAttr: []string{"group:intel-stress"},
			Timeout:   time.Hour * 2,
		}, {
			Name: "tabletmode",
			Val: batteryStatusTestParam{
				iter:       1,
				tabletMode: true,
			},
			Timeout:   time.Hour * 2,
			ExtraAttr: []string{"group:intel-convertible"},
		},
		}})
}

func BatteryStatusOnACRemoval(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dut := s.DUT()

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	chargerPollOptions := testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: 250 * time.Millisecond,
	}

	testOpts := s.Param().(batteryStatusTestParam)

	if testOpts.tabletMode {
		// Get the initial tablet_mode_angle settings to restore at the end of test.
		re := regexp.MustCompile(`tablet_mode_angle=(\d+) hys=(\d+)`)
		out, err := dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle").Output()
		if err != nil {
			s.Fatal("Failed to retrieve tablet_mode_angle settings: ", err)
		}
		m := re.FindSubmatch(out)
		if len(m) != 3 {
			s.Fatalf("Failed to get initial tablet_mode_angle settings: got submatches %+v", m)
		}
		tabletModeAngle := m[1]
		hys := m[2]
		// Set tabletModeAngle to 0 to force the DUT into tablet mode.
		testing.ContextLog(ctx, "Put DUT into tablet mode")
		if err := dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", "0", "0").Run(); err != nil {
			s.Fatal("Failed to set DUT into tablet mode: ", err)
		}
		defer func(ctx context.Context) {
			if err := dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", string(tabletModeAngle), string(hys)).Run(); err != nil {
				s.Fatal("Failed to restore tablet_mode_angle to the original settings: ", err)
			}
		}(cleanupCtx)
	} else {
		if err := ensureClamshellMode(ctx, h, dut); err != nil {
			s.Fatal("Failed to enusre that the DUT is in clamshell mode: ", err)
		}

	}
	cl, err := rpc.Dial(ctx, h.DUT, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	client := power.NewBatteryServiceClient(cl.Conn)
	if _, err := client.New(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer client.Close(cleanupCtx, &empty.Empty{})

	defer func(ctx context.Context) {
		s.Log("Test defer, finishing: Plugging power supply")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Error("Failed to connect charger: ", err)
		}
	}(cleanupCtx)

	iterations := testOpts.iter
	for i := 1; i <= iterations; i++ {
		s.Logf("Iteration: %d/%d", i, iterations)
		//Checking initial battery charge.
		initialCharge, err := getChargePercent(ctx, h)
		if err != nil {
			s.Fatal("Failed to get battery level: ", err)
		}
		// Putting battery within testable range.

		targetDischarge := initialCharge
		targetCharge := initialCharge + 3

		if initialCharge >= 92 {
			// Handle the case where we need some headroom to charge/discharge
			targetCharge = 95
			targetDischarge = 92

			s.Log("Stopping power supply")
			if err := h.SetDUTPower(ctx, false); err != nil {
				s.Fatal("Failed to remove charger: ", err)
			}
			request := power.BatteryRequest{MaxPercentage: float32(targetDischarge)}
			if _, err := client.DrainBattery(ctx, &request); err != nil {
				s.Fatal("Failed to drain battery: ", err)
			}

		}

		s.Log("Plugging power supply")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to connect charger: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if attached, err := h.Servo.GetChargerAttached(ctx); err != nil {
				return err
			} else if !attached {
				return errors.New("charger is not attached - use Servo V4 Type-C or supply RPM vars")
			}
			return nil
		}, &chargerPollOptions); err != nil {
			s.Fatal("Failed to check if charger is connected via Servo V4: ", err)
		}

		// Verifying battery charging with power_supply_info command.
		s.Log("Checking battery information for charging")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if charging, err := isBatteryCharging(ctx, h); err != nil {
				return errors.Wrap(err, "failed to verify battery information")
			} else if !charging {
				return errors.New("failed to verify expected charging status")
			}
			return nil
		}, &chargerPollOptions); err != nil {
			s.Fatal("Failed to check charging status from power_supply_info: ", err)
		}

		// Charging the DUT for 3%.
		s.Logf("Waiting for battery to reach %d%%", targetCharge)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			pct, err := getChargePercent(ctx, h)
			if err != nil {
				// Failed to get battery level so stop trying.
				return testing.PollBreak(err)
			}
			if pct < targetCharge {
				return errors.Errorf("Current battery charge is %d%%, required %d%%", pct, targetCharge)
			}

			return nil
		}, &testing.PollOptions{Timeout: time.Hour, Interval: time.Minute}); err != nil {
			s.Fatal("Failed to charge DUT: ", err)
		}

		s.Log("Stopping power supply")
		if err := h.SetDUTPower(ctx, false); err != nil {
			s.Fatal("Failed to remove charger: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if attached, err := h.Servo.GetChargerAttached(ctx); err != nil {
				return err
			} else if attached {
				return errors.New("charger is still attached - use Servo V4 Type-C or supply RPM vars")
			}
			return nil
		}, &chargerPollOptions); err != nil {
			s.Fatal("Failed to check if charger is disconnected via Servo V4: ", err)
		}

		// Discharging DUT for 3%.
		s.Logf("Discharging DUT till %d%%", targetDischarge)
		request := power.BatteryRequest{MaxPercentage: float32(targetDischarge)}
		if _, err := client.DrainBattery(ctx, &request); err != nil {
			s.Fatal("Failed to drain battery: ", err)
		}

		// Verifying battery discharging with power_supply_info command.
		s.Log("Checking battery information for charging")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if charging, err := isBatteryCharging(ctx, h); err != nil {
				return errors.Wrap(err, "failed to verify battery information")
			} else if charging {
				return errors.New("failed to verify expected charging status")
			}
			return nil
		}, &chargerPollOptions); err != nil {
			s.Fatal("Failed to check charging status from power_supply_info: ", err)
		}
	}
}

// isBatteryCharging returns true if battery is charging.
func isBatteryCharging(ctx context.Context, h *firmware.Helper) (bool, error) {
	regex := `state:(\s+\w+\s?\w+)`
	expMatch := regexp.MustCompile(regex)

	out, err := h.DUT.Conn().CommandContext(ctx, "power_supply_info").Output()
	if err != nil {
		return false, errors.Wrap(err, "failed to retrieve power supply info from DUT")
	}

	matches := expMatch.FindStringSubmatch(string(out))
	if len(matches) < 2 {
		return false, errors.Errorf("failed to match regex %q in %q", expMatch, string(out))
	}

	return strings.TrimSpace(matches[1]) != "Discharging", nil
}

// getChargePercent returns battery charge percentage.
func getChargePercent(ctx context.Context, h *firmware.Helper) (int, error) {
	var err error = nil
	currentMAH := 0
	maxMAH := 0
	testing.Poll(ctx, func(ctx context.Context) error {
		currentMAH, err = h.Servo.GetBatteryChargeMAH(ctx)
		if err != nil {
			return err
		}
		maxMAH, err = h.Servo.GetBatteryFullChargeMAH(ctx)
		if err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second, Interval: time.Second})
	if err != nil {
		return -1, err
	}

	return int(100 * float32(currentMAH) / float32(maxMAH)), nil
}

// ensureClamshellMode checks whether DUT is in tablet mode.
func ensureClamshellMode(ctx context.Context, h *firmware.Helper, dut *dut.DUT) error {
	inTabletMode, err := checkTabletModeStatus(ctx, h)
	if err != nil {
		return errors.Wrap(err, "unable to check for DUT's tablet mode status")
	}
	if inTabletMode {
		testing.ContextLog(ctx, "DUT is in tablet mode. Attempting to turn tablet mode off")
		_, err := h.Servo.RunTabletModeCommandGetOutput(ctx, "tabletmode off")
		if err != nil {
			testing.ContextLogf(ctx, "Failed to run tabletmode_off: %v. Attempting to set rotation angles with ectool instead", err)
			ecToolCmd := firmware.NewECTool(dut, firmware.ECToolNameMain)
			// Setting tabletModeAngle to 360 will force DUT into clamshell mode.
			if err := ecToolCmd.ForceTabletModeAngle(ctx, "360", "0"); err != nil {
				return errors.Wrap(err, "failed to set DUT in clamshell mode")
			}
		}
	}
	return nil
}

// checkTabletModeStatus checks whether DUT is in tablet mode through the utils service.
func checkTabletModeStatus(ctx context.Context, h *firmware.Helper) (bool, error) {
	if err := h.RequireRPCUtils(ctx); err != nil {
		return false, errors.Wrap(err, "requiring RPC utils")
	}
	// GoBigSleepLint: Sleeping for a few seconds before starting a new Chrome.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return false, errors.Wrap(err, "failed to wait for a few seconds")
	}
	if _, err := h.RPCUtils.NewChrome(ctx, &empty.Empty{}); err != nil {
		return false, errors.Wrap(err, "failed to create instance of chrome")
	}
	defer h.RPCUtils.CloseChrome(ctx, &empty.Empty{})
	res, err := h.RPCUtils.EvalTabletMode(ctx, &empty.Empty{})
	if err != nil {
		return false, errors.Wrap(err, "failed to evaluate tablet mode")
	}
	return res.TabletModeEnabled, nil
}
