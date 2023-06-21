// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Note: This test makes use of "modular servo" to connect ServoV4.x, Cr50, and
// the CPD (cros power debugger) to the device under test. This allows us to
// use a single servod instance to measure low power, and then wake the DUT back
// up after the test completes. This means that the user /must/ set up the servod
// instance as
//
// servod --board=$BOARD --device-discovery=full
//
// Note: This test also handles the inherent stabilization time necessary byt
// logging the low-power state after 25 minutes of test time. Future improvements to
// this logic and documentation of the behavior are tracked in (b/277269491)

const (
	varLPMDuration     = "duration"
	varServoHostPort   = "servoHostPort"
	defaultLPMDuration = 30 * time.Minute
	chargeTarget       = 99.

	// use [len-stableSamples:] to determine consumption
	stableSamples = 60

	// sample every [sampleTime]
	sampleTime = 5 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LowPowerConsumption,
		Desc: "Places the device in a low power mode, measures consumption with CPD",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
		},
		BugComponent: "b:1110659", // ChromeOS > Platform > Enablement > PVS Framework
		Attr:         []string{"group:power", "power_cpd"},
		Fixture:      fixture.NormalMode,
		Timeout:      defaultLPMDuration + 2*time.Hour,           // Ensure we have enough time for test setup/teardown
		Vars:         []string{varLPMDuration, varServoHostPort}, // Duration is in seconds, servoHostPort defaults 9999
		Params: []testing.Param{
			{
				Name:              "suspend",
				Val:               "suspend",
				ExtraRequirements: []string{"pwr-batLife-0007-v01", "pwr-batLife-0007-v02"},
			},
			{
				Name:              "shutdown",
				Val:               "shutdown",
				ExtraRequirements: []string{"pwr-batLife-0004-v01", "pwr-batLife-0005-v01"},
			},
		},
	})
}

func LowPowerConsumption(ctx context.Context, s *testing.State) {
	// Fixture helper from "NormalMode" fixture
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	duration := defaultLPMDuration
	if v, ok := s.Var(varLPMDuration); ok {
		seconds, err := strconv.Atoi(v)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s: %v", varLPMDuration, v, err)
		}
		duration = time.Duration(seconds) * time.Second
	}
	// Can't exceed defaultDuration because the test will timeout
	if duration > defaultLPMDuration {
		s.Fatalf("Duration too long, got %s, wanted <= %s", duration, defaultLPMDuration)
	}
	s.Logf("Suspend duration %s", duration)

	// Get target state for entrance map
	targetState := s.Param().(string)

	// Ensure that the DUT has a charged battery
	chargeBattery(ctx, s)
	capacity, err := getCapacity(ctx, s.DUT())
	s.Logf("Charged Ah capacity of battery is  %f", capacity)

	// Make sure that the watchdog on ccd is disconnected
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to switch CCD watchdog off: ", err)
	}

	// execute the entranceTable for the state in param
	err = entranceTable[targetState](ctx, s.DUT(), h)
	if err != nil {
		s.Fatal("Could not execute entrance function: ", err)
	}

	// wait for the device's state to stabilize
	// GoBigSleepLint: this is a known/static interval of time, after
	// which we poll the device's state continuously
	testing.Sleep(ctx, 10*time.Second)

	// loop and get accumulator output every 5 seconds, get accumulated mw
	// and calc approx avg mA from the output
	endTime := time.Now().Add(duration)
	err = h.Servo.SetInt(ctx, "ft4232h_generic.CPD_VBAT_acc_clear", 1)
	if err != nil {
		s.Fatalf("Failed to clear accumulator: %s", err)
	}
	mwSamples := make([]float64, 0)
	maSamples := make([]float64, 0)
	lifetimes := make([]float64, 0)
	sampleCount := make([]int64, 0)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		mw, err := h.Servo.GetFloat(ctx, "ft4232h_generic.CPD_VBAT_avg_mw")
		if err != nil {
			return errors.Errorf("failed to get mw from servo instance: %s", err)
		}
		mv, err := h.Servo.GetFloat(ctx, "ft4232h_generic.CPD_VBAT_mv")
		if err != nil {
			return errors.Errorf("failed to get mv from servo instance: %s", err)
		}
		samples, err := h.Servo.GetString(ctx, "ft4232h_generic.CPD_VBAT_acc_count_reg")
		if err != nil {
			return errors.Errorf("failed to get acc count from servo instance: %s", err)
		}
		samples = strings.Replace(samples, "0x", "", -1)
		samplesInt, err := strconv.ParseInt(samples, 16, 64)
		if err != nil {
			return errors.Errorf("failed to convert sample count: %s", err)
		}

		// clear the accumulator at the end of the loop, so that during
		// the interval we're accumulating
		err = h.Servo.SetInt(ctx, "ft4232h_generic.CPD_VBAT_acc_clear", 1)
		if err != nil {
			return errors.Errorf("failed to clear accumulator: %s", err)
		}

		amps := mw / mv
		ma := amps * 1000
		approxLife := (capacity / amps) / 24
		s.Logf("mW: %f, approx mA: %f, approx lifetime (days): %f, samples %d", mw, ma, approxLife, samplesInt)

		mwSamples = append(mwSamples, mw)
		maSamples = append(maSamples, ma)
		lifetimes = append(lifetimes, approxLife)
		sampleCount = append(sampleCount, samplesInt)

		if time.Now().Before(endTime) {
			return errors.New("timeout is not complete")
		}
		return nil
	}, &testing.PollOptions{Timeout: duration + 1*time.Minute, Interval: sampleTime}); err != nil {
		s.Fatal("Failed to complete polling low power mode: ", err)
	}
	lpmPerf := perf.NewValues()
	lpmPerf.Set(perf.Metric{
		Name:      "mw",
		Unit:      "milliwatts",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, mwSamples...)
	lpmPerf.Set(perf.Metric{
		Name:      "ma",
		Unit:      "milliamps",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, maSamples...)

	// constrain samples used for stable lifetime estimation to the length
	// of the overall
	numStableSamples := stableSamples
	if numStableSamples > len(lifetimes) {
		numStableSamples = len(lifetimes)
	}
	averagedSamples := 0
	totalCount := sampleCount[len(lifetimes)-1]
	runningAverage := 0.
	for sample := len(lifetimes) - 1; averagedSamples < numStableSamples-1; sample-- {
		newSampleRatio := float64(sampleCount[sample]) / float64(totalCount)
		runningAverageRatio := 1. - newSampleRatio
		runningAverage = (lifetimes[sample] * newSampleRatio) + (runningAverage * runningAverageRatio)

		averagedSamples++
		totalCount += sampleCount[sample]
	}
	lpmPerf.Set(perf.Metric{
		Name:      "lifetime",
		Unit:      "days",
		Direction: perf.BiggerIsBetter,
	}, runningAverage)

	if err := lpmPerf.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}

	// execute the exitTable for the state in param
	err = exitTable[targetState](ctx, s.DUT(), h)
	if err != nil {
		s.Fatal("Could not execute exit function: ", err)
	}

	// Make sure that the watchdog on ccd is re-enabled
	if err := h.Servo.WatchdogAdd(ctx, servo.WatchdogCCD); err != nil {
		s.Fatal("Failed to switch CCD watchdog on: ", err)
	}
}

func chargeBattery(ctx context.Context, s *testing.State) {
	s.Logf("Waiting for battery to reach %f", chargeTarget)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := s.DUT().Conn().CommandContext(ctx, "power_supply_info").Output()
		if err != nil {
			return errors.Wrap(err, "failed to get power_supply_info")
		}
		fullChargeRe := regexp.MustCompile(`.*display percentage:\s*([0-9.]*)`)
		matches := fullChargeRe.FindSubmatch(out)
		if len(matches) != 2 {
			return errors.Wrap(err, "display percentage not found in power_supply_info")
		}
		if percent, err := strconv.ParseFloat(string(matches[1]), 64); err != nil {
			return err
		} else if percent < chargeTarget {
			s.Logf("charging, percentage is %f percent", percent)
			return errors.Errorf("display percentage %f, want >%f", percent, chargeTarget)
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Minute, Interval: 5 * time.Second}); err != nil {
		s.Fatal("Failed to finish charging battery: ", err)
	}
}

// getCapacity gets battery capacity information from power_supply_info,
// returns the value in amp hours
func getCapacity(ctx context.Context, dut *dut.DUT) (float64, error) {
	out, err := dut.Conn().CommandContext(ctx, "power_supply_info").Output()
	if err != nil {
		return 0, errors.Wrap(err, "failed to get power_supply_info")
	}
	fullChargeRe := regexp.MustCompile(`.*full charge \(Ah\):\s*([0-9.]*)`)
	matches := fullChargeRe.FindSubmatch(out)
	if len(matches) != 2 {
		return 0, errors.Wrap(err, "full charge (Ah) not found in power_supply_info")
	}
	return strconv.ParseFloat(string(matches[1]), 64)
}

// entranceTable is a map to the entrance functions for different states
var entranceTable = map[string]func(ctx context.Context, dut *dut.DUT, helper *firmware.Helper) error{
	"suspend": func(ctx context.Context, dut *dut.DUT, helper *firmware.Helper) error {
		if err := helper.Servo.SetOnOff(ctx, servo.CCDKeepaliveEn, servo.Off); err != nil {
			return errors.Errorf("failed to disable CCD keepalive: %s", err)
		}
		// first, suspend the device (with delay)
		pwrDbusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := dut.Conn().CommandContext(pwrDbusCtx, "powerd_dbus_suspend", "--delay=10").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return errors.Errorf("failed to set powerd_dbus_suspend: %s", err)
		}
		// during the delay, unplug charger
		if err := helper.Servo.SetCC(ctx, "off"); err != nil {
			return errors.Errorf("unable to disable Servo CC: %s", err)
		}
		return nil
	},
	"shutdown": func(ctx context.Context, dut *dut.DUT, helper *firmware.Helper) error {
		if err := helper.Servo.SetOnOff(ctx, servo.CCDKeepaliveEn, servo.Off); err != nil {
			return errors.Errorf("failed to disable CCD keepalive: %s", err)
		}
		hibdelayCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := dut.Conn().CommandContext(hibdelayCtx, "ectool", "hibdelay", "5").Run(); err != nil {
			return errors.Wrap(err, "failed to set hibdelay")
		}
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := dut.Conn().CommandContext(shutdownCtx, "shutdown", "-h", "now").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return errors.Wrap(err, "failed to shutdown")
		}
		// unplug charger
		if err := helper.Servo.SetCC(ctx, "off"); err != nil {
			return errors.Errorf("unable to disable Servo CC: %s", err)
		}
		sdCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		if err := dut.WaitUnreachable(sdCtx); err != nil {
			return errors.Wrap(err, "failed to wait for unreachable")
		}
		return nil
	},
}

// exitTable is a map to the exit functions for different states
var exitTable = map[string]func(ctx context.Context, dut *dut.DUT, helper *firmware.Helper) error{
	"suspend": func(ctx context.Context, dut *dut.DUT, helper *firmware.Helper) error {
		// just re-plug the connector to power on
		if err := helper.Servo.SetCC(ctx, "srcdts"); err != nil {
			return errors.Errorf("failed to enable Servo CC: %s", err)
		}
		// GoBigSleepLint this sleep is necessary to allow the charger to reconnect.
		// there is *no way* to poll the following function without crashing uart.
		testing.Sleep(ctx, 1*time.Second)

		if err := helper.Servo.SetOnOff(ctx, servo.CCDKeepaliveEn, servo.On); err != nil {
			return errors.Errorf("failed to enable CCD keepalive: %s", err)
		}
		return nil
	},
	"shutdown": func(ctx context.Context, dut *dut.DUT, helper *firmware.Helper) error {
		// just re-plug the connector to power on
		if err := helper.Servo.SetCC(ctx, "srcdts"); err != nil {
			return errors.Errorf("failed to enable Servo CC: %s", err)
		}
		// GoBigSleepLint this sleep is necessary to allow the charger to reconnect.
		// there is *no way* to poll the following function without crashing uart.
		testing.Sleep(ctx, 1*time.Second)

		if err := helper.Servo.SetOnOff(ctx, servo.CCDKeepaliveEn, servo.On); err != nil {
			return errors.Errorf("failed to enable CCD keepalive: %s", err)
		}
		// using servo to wake up the DUT
		return powercontrol.PowerOntoDUT(ctx, helper.ServoProxy, dut)
	},
}
