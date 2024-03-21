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
	"go.chromium.org/tast-tests/cros/remote/tabletmode"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/ctxutil"
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
				iter:       1,
				tabletMode: false,
			},
			ExtraAttr: []string{"group:intel-stress"},
			Timeout:   time.Minute * 30,
		}, {
			Name: "tabletmode",
			Val: batteryStatusTestParam{
				iter:       1,
				tabletMode: true,
			},
			Timeout:   time.Minute * 30,
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
	tmc := &tabletmode.ConvertibleModeControl{}
	if err := tmc.InitControl(ctx, dut); err != nil {
		s.Fatal("Failed to init TabletModeControl: ", err)
	}

	testOpts := s.Param().(batteryStatusTestParam)
	if testOpts.tabletMode {
		testing.ContextLog(ctx, "Put DUT into tablet mode")
		if err := tmc.ForceTabletMode(ctx); err != nil {
			s.Fatal("Failed to set DUT into tablet mode: ", err)
		}

		defer func(ctx context.Context) {
			testing.ContextLog(ctx, "Resetting tabletmode")
			if err := tmc.Reset(ctx); err != nil {
				s.Fatal("Failed to restore tabletmode to the original settings: ", err)
			}
		}(cleanupCtx)
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
		// Checking initial battery charge.
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
			minPercentage := float32(targetDischarge - 1)
			if minPercentage < 0 {
				minPercentage = 0.0
			}
			request := power.BatteryRequest{
				MinPercentage:         minPercentage,
				MaxPercentage:         float32(targetDischarge),
				DischargeOnCompletion: false,
			}
			if _, err := client.PrepareBattery(ctx, &request); err != nil {
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
		request := power.BatteryRequest{
			MinPercentage:         float32(targetCharge),
			MaxPercentage:         float32(targetCharge),
			DischargeOnCompletion: false,
		}
		if _, err := client.PrepareBattery(ctx, &request); err != nil {
			s.Fatal("Failed to charge battery: ", err)
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
		request = power.BatteryRequest{
			MinPercentage:         float32(targetDischarge),
			MaxPercentage:         float32(targetDischarge),
			DischargeOnCompletion: false,
		}
		if _, err := client.PrepareBattery(ctx, &request); err != nil {
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
