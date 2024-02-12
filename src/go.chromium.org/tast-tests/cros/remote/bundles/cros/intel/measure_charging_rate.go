// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type chargingRateTestParam struct {
	isIdleMode bool
}

const (
	requiredBatteryPercent = 70
	maxBatteryPercent      = 100.00
	chargeCheckInterval    = time.Minute
	chargeCheckTimeout     = 90 * time.Minute
	batteryLevelTimeout    = 20 * time.Second // default servo comm timeout is 10s, battery check requires two.
	batteryLevelInterval   = time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MeasureChargingRate,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measuring charging rate in suspend mode (S0ix)",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		ServiceDeps:  []string{"tast.cros.power.BatteryService"},
		SoftwareDeps: []string{"chrome", "crossystem"},
		HardwareDeps: hwdep.D(hwdep.Battery()),
		Fixture:      fixture.NormalMode,
		Attr:         []string{"group:intel-stress"},
		Params: []testing.Param{{
			Name: "s0ix",
			Val: chargingRateTestParam{
				isIdleMode: false,
			},
			Timeout: 120 * time.Minute,
		}, {
			Name: "idle",
			Val: chargingRateTestParam{
				isIdleMode: true,
			},
			Timeout: 120 * time.Minute,
		},
		}})
}

func MeasureChargingRate(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	dut := s.DUT()
	testOpts := s.Param().(chargingRateTestParam)

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
		s.Log("Plugging power supply")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Error("Failed to connect charger: ", err)
		}
	}(cleanupCtx)

	// Checking initial battery charge.
	initialCharge, err := getChargePercentage(ctx, h)
	if err != nil {
		s.Fatal("Failed to get battery level: ", err)
	}
	// Putting battery within testable range.
	if initialCharge >= requiredBatteryPercent {
		s.Logf("Current charge is %v; Required charge is %v; Stopping power supply & draining battery", initialCharge, requiredBatteryPercent)
		if err := h.SetDUTPower(ctx, false); err != nil {
			s.Fatal("Failed to remove charger: ", err)
		}
		minPercentage := float32(requiredBatteryPercent - 1)
		if minPercentage < 0 {
			minPercentage = 0
		}
		request := power.BatteryRequest{
			MinPercentage:         minPercentage,
			MaxPercentage:         requiredBatteryPercent,
			DischargeOnCompletion: true,
		}
		if _, err := client.PrepareBattery(ctx, &request); err != nil {
			s.Fatal("Failed to drain battery: ", err)
		}
	}

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
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 250 * time.Millisecond}); err != nil {
		s.Fatal("Failed to check if charger is connected via Servo V4: ", err)
	}

	s.Log("Performing cold reboot")
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to perform mode aware reboot: ", err)
	}

	newCl, err := rpc.Dial(ctx, h.DUT, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer newCl.Close(cleanupCtx)

	newClient := power.NewBatteryServiceClient(newCl.Conn)
	if _, err := newClient.New(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer newClient.Close(cleanupCtx, &empty.Empty{})

	if testOpts.isIdleMode {
		brightness, err := brightnessPercent(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get system brightness: ", err)
		}

		if brightness != 40 {
			if err := setBrightnessPercent(ctx, 40, dut); err != nil {
				s.Fatal("Failed to set required brightness: ", err)
			}
		}

		if _, err := newClient.PowerSettingInIdleMode(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to perform power setting in idle mode: ", err)
		}

		// GoBigSleepLint: Charging DUT for 1 minute when dut is idle.
		if err := testing.Sleep(ctx, 1*time.Minute); err != nil {
			s.Fatal("Failed to be in idle mode: ", err)
		}

		startTime := time.Now()
		chargeBeforeSleep, err := getChargePercentage(ctx, h)
		if err != nil {
			s.Fatal("Failed to get battery level: ", err)
		}

		s.Log("Charging DUT for 5 minutes when dut is idle")
		// GoBigSleepLint: For 5 minutes, observe battery charging status.
		if err := testing.Sleep(ctx, 5*time.Minute); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		// Check battery charge after 10 minutes.
		chargeAfterSleep, err := getChargePercentage(ctx, h)
		if err != nil {
			s.Fatal("Failed to get battery level: ", err)
		}
		endTime := time.Now()

		totalTime := endTime.Sub(startTime) * 100 / time.Duration((chargeAfterSleep-chargeBeforeSleep)*float32(time.Minute))
		s.Log("Total Time to Full Charge in minutes: ", totalTime)
		if totalTime > 180 {
			s.Fatal("Failed: Total battery charging time is more than 3 hours")
		}

		s.Logf("Waiting for battery to reach %v%%", maxBatteryPercent)
		if err := waitForCharge(ctx, h, maxBatteryPercent); err != nil {
			s.Fatalf("Failed to reach target %v%%, %v", maxBatteryPercent, err.Error())
		}

	} else {
		slpOpSetPre, pkgOpSetPre, err := powercontrol.SlpAndC10PackageValues(ctx, h.DUT)
		if err != nil {
			s.Fatal("Failed to get SLP counter and C10 package values before suspend-resume: ", err)
		}

		// Emulate DUT lid closing.
		if err := h.Servo.CloseLid(ctx); err != nil {
			s.Fatal("Failed to close DUT's lid: ", err)
		}

		testing.Poll(ctx, func(ctx context.Context) error {
			s.Log("Checking lid state after closing lid")
			lidState, err := h.Servo.LidOpenState(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to check the final lid state")
			}
			if lidState != string(servo.LidOpenNo) {
				return errors.Errorf("failed to check DUT lid state, got %q, want %q", lidState, servo.LidOpenNo)

			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second})

		startTime := time.Now()
		chargeBeforeSleep, err := getChargePercentage(ctx, h)
		if err != nil {
			s.Fatal("Failed to get battery level: ", err)
		}

		s.Log("Charging DUT for 10 minutes after closing lid")
		// GoBigSleepLint: For 10 minutes, observe battery charging status.
		if err := testing.Sleep(ctx, 10*time.Minute); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		// Check battery charge after 10 minutes.
		chargeAfterSleep, err := getChargePercentage(ctx, h)
		if err != nil {
			s.Fatal("Failed to get battery level: ", err)
		}
		endTime := time.Now()

		totalTime := endTime.Sub(startTime) * 100 / time.Duration((chargeAfterSleep-chargeBeforeSleep)*float32(time.Minute))
		s.Log("Total Time to Full Charge in minutes: ", totalTime)
		if totalTime > 180 {
			s.Fatal("Failed: Total battery charging time is more than 3 hours")
		}

		s.Logf("Waiting for battery to reach %v%%", maxBatteryPercent)
		if err := waitForCharge(ctx, h, maxBatteryPercent); err != nil {
			s.Fatalf("Failed to reach target %v%%, %v", maxBatteryPercent, err.Error())
		}
		// Emulate DUT lid opening.
		if err := h.Servo.OpenLid(ctx); err != nil {
			s.Fatal("Failed to open DUT's lid: ", err)
		}
		testing.Poll(ctx, func(ctx context.Context) error {
			s.Log("Checking lid state after opening lid")
			lidState, err := h.Servo.LidOpenState(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to check the final lid state")
			}
			if lidState != string(servo.LidOpenYes) {
				return errors.Errorf("failed to check DUT lid state, got %q, want %q", lidState, servo.LidOpenYes)

			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second})

		waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if err := h.DUT.WaitConnect(waitCtx); err != nil {
			s.Fatal("Failed to wait connect DUT: ", err)
		}

		slpOpSetPost, pkgOpSetPost, err := powercontrol.SlpAndC10PackageValues(ctx, h.DUT)
		if err != nil {
			s.Fatal("Failed to get SLP counter and C10 package values after suspend-resume: ", err)
		}

		if err := powercontrol.AssertSLPAndC10(slpOpSetPre, slpOpSetPost, pkgOpSetPre, pkgOpSetPost); err != nil {
			s.Fatal("Failed to verify SLP and C10 state values: ", err)
		}
	}
}

// waitForCharge charges the DUT to required charge percentage.
func waitForCharge(ctx context.Context, h *firmware.Helper, target float32) error {
	// Make sure AC power is connected.
	// The original setting will be restored automatically when the test ends.
	if err := h.SetDUTPower(ctx, true); err != nil {
		return errors.Wrap(err, "failed to set DUT power")
	}

	err := testing.Poll(ctx, func(ctx context.Context) error {
		pct, err := getChargePercentage(ctx, h)
		if err != nil {
			// Failed to get battery level so stop trying.
			return testing.PollBreak(err)
		}

		if pct < target {
			return errors.Errorf("Current battery charge is %v%%, required %v%%", pct, target)
		}

		return nil
	}, &testing.PollOptions{Timeout: chargeCheckTimeout, Interval: chargeCheckInterval})

	if err != nil {
		return errors.Wrap(err, "failed to get charge percentage")
	}

	// Disable Servo power to DUT.
	if err := h.SetDUTPower(ctx, false); err != nil {
		return errors.Wrap(err, "failed to set DUT power")
	}

	return nil
}

// getChargePercentage returns the current charge of the battery.
func getChargePercentage(ctx context.Context, h *firmware.Helper) (float32, error) {

	// Attempt to determine the battery percentage.
	// Each servo communication attempt is retried to account for any transient
	// communication problems.
	var err error = nil
	currentMAH := 0
	maxMAH := 0

	testing.Poll(ctx, func(ctx context.Context) error {
		currentMAH, err = h.Servo.GetBatteryChargeMAH(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get battery charge mAh")

		}
		maxMAH, err = h.Servo.GetBatteryFullChargeMAH(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get battery full charge mAh")

		}
		return nil
	}, &testing.PollOptions{Timeout: batteryLevelTimeout, Interval: batteryLevelInterval})

	if err != nil {
		return -1, errors.Wrap(err, "failed to get battery charge details")
	}
	return 100 * float32(currentMAH) / float32(maxMAH), nil
}

// brightnessPercent gets the current brightness of the system.
func brightnessPercent(ctx context.Context, dut *dut.DUT) (float64, error) {
	out, err := dut.Conn().CommandContext(ctx, "backlight_tool", "--get_brightness_percent").Output()
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to execute brightness command")
	}
	sysBrightness, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to parse string into float64")
	}
	return sysBrightness, nil
}

// setBrightnessPercent sets the brightness of the system.
func setBrightnessPercent(ctx context.Context, percent float64, dut *dut.DUT) error {
	if err := dut.Conn().CommandContext(ctx, "backlight_tool", fmt.Sprintf("--set_brightness_percent=%f", percent)).Run(); err != nil {
		return errors.Wrapf(err, "failed to set %f%% brightness", percent)
	}
	return nil
}
