// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/servod"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/tastrun"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

/**
Example:
tast run -var "subtest=power.ExampleUI.ash_kbbl" $DUT_IP meta.PowerServodWrapper.cpd_manual

*/

type testParams struct {
	// filter is applied when first finding servod rails.
	filter string
	// subtest specifies the test to be run within.
	// If provided, the command line subtest will overwrite testParams.
	subtest string
}

var servoPowerMeasureIntervalVar = testing.RegisterVarString(
	"meta.PowerServodWrapper.interval",
	defaultServoPowerMeasureInterval,
	"interval defines seconds between servo power measurements",
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerServodWrapper,
		Desc:         "Runs test while capturing power data using servod",
		LacrosStatus: testing.LacrosVariantUnneeded,
		BugComponent: "b:167191", // ChromeOS > Platform > System > Power
		Contacts:     []string{"cros-pe-pnp@google.com", "khwon@google.com"},
		Timeout:      24 * time.Hour, // Depends on subtest, so set maximum value here.
		Params: []testing.Param{
			// Special test cases can be added as a Param here.
			{
				Name: "manual",
				Val:  testParams{},
			},
			{
				Name: "cpd_manual",
				Val: testParams{
					filter: cpdFilter,
				},
			},
			{
				Name: "cpd_vp_h264_1080_30fps",
				Val: testParams{
					filter:  cpdFilter,
					subtest: "power.VideoPlayback.h264_1080_30fps_ash",
				},
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
			{
				Name: "cpd_vp_vp9_1080_30fps",
				Val: testParams{
					filter:  cpdFilter,
					subtest: "power.VideoPlayback.vp9_1080_30fps_ash",
				},
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
		},
		Vars: []string{"servo", "subtest"},
	})
}

const (
	// defaultServoPowerMeasureInterval in seconds.
	defaultServoPowerMeasureInterval = "2"
	chargeTarget                     = 75.
	intervalMetricName               = "t"
	cpdFilter                        = "ft4232h_generic"
)

func PowerServodWrapper(ctx context.Context, s *testing.State) {
	resultsDir := filepath.Join(s.OutDir(), "subtest_results")

	// servoCtx is used for async function measuring power.
	servoCtx, servoCancel := context.WithCancel(ctx)

	cleanupCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	param, ok := s.Param().(testParams)
	if !ok {
		s.Fatal("Failed to convert test testParams")
	}

	// Determine subtest, prefer command line subtest.
	var subtest string
	varTest, ok := s.Var("subtest")

	if !ok {
		if param.subtest == "" {
			s.Fatal("Please specify a subtest or use a pre-defined subtest")
		}
		subtest = param.subtest

	} else {
		subtest = varTest
	}

	s.Log("Subtest: ", subtest)

	// Determine Servo measurement interval.
	intervalStrVal := servoPowerMeasureIntervalVar.Value()
	intervalIntVal, err := strconv.Atoi(intervalStrVal)
	if err != nil || intervalIntVal <= 0 {
		s.Fatal("Failed to parse meta.PowerServodWrapper.interval: ", err)
	}
	servoPowerMeasureInterval := time.Duration(intervalIntVal) * time.Second

	// Connect to Servo.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctx)

	// Query for available accumulator rails.
	var sregex *regexp.Regexp
	if param.filter != "" {
		sregex = regexp.MustCompile(param.filter)
	}
	rails, clearRails, err := servo.FindAccumRailsWithFilter(ctx, pxy.Servo(), sregex)
	if err != nil {
		s.Fatal("Failed to get accum rails: ", err)
	}
	s.Log("Avg power rail commands found:", rails)

	chargeBattery(cleanupCtx, s)

	// Disable charging.
	if _, err := s.DUT().Conn().CommandContext(cleanupCtx, "ectool", "chargeoverride", "dontcharge").Output(); err != nil {
		s.Fatal("Unable to disable charging: ", err)
	}

	intervalMetric := perf.Metric{
		Name:      intervalMetricName,
		Unit:      "s",
		Multiple:  true,
		Direction: perf.SmallerIsBetter,
	}

	ch := make(chan *perf.Values)
	// TODO: b/304656798 - Investigate timestamps.
	measureStarted := float64(time.Now().Unix())
	// TODO: b/304655966 - Implement timeline interface.
	go func() {
		if err = servo.ClearServoAccumulators(ctx, pxy.Servo(), clearRails); err != nil {
			s.Fatal("Unable to clear servo accumulators: ", err)
		}
		pv := perf.NewValues()
		for {
			select {
			case <-time.After(servoPowerMeasureInterval):

				for _, railMw := range rails {
					mw, err := pxy.Servo().GetFloat(ctx, railMw)
					if err != nil {
						s.Fatalf("Failed to get %s mw from servo instance: %s", string(railMw), err)
					}
					pv.Append(perf.Metric{
						Name:      string(railMw),
						Unit:      "mW",
						Direction: perf.SmallerIsBetter,
						Multiple:  true,
						Interval:  intervalMetricName,
					}, mw)
				}
				// Clear the accumulator at the end of the loop to measure the interval.
				if err = servo.ClearServoAccumulators(ctx, pxy.Servo(), clearRails); err != nil {
					s.Fatal("Unable to clear servo accumulators: ", err)
				}
				pv.Append(intervalMetric, float64(time.Now().Unix()))
			case <-servoCtx.Done():
				ch <- pv
				return
			}
		}
	}()

	s.Log("Starting subtest: ", subtest)
	skippedTests := tastrun.RunAndEvaluate(cleanupCtx, s, []string{}, []string{subtest}, resultsDir, tastrun.SkipPolicyDisallowSkipping)
	if len(skippedTests) > 0 {
		s.Fatal("Test is skipped, abort post-processing")
	} else {
		s.Log("Finished subtest: ", subtest)
	}

	servoCancel()
	servoResult := <-ch

	// Enable charging.
	// TODO: b/303548068 - Sync CC with setup_battery.go.
	if _, err := s.DUT().Conn().CommandContext(cleanupCtx, "ectool", "chargeoverride", "off").Output(); err != nil {
		s.Fatal("Unable to enable charging: ", err)
	}

	subtestDir := filepath.Join(resultsDir, "tests", subtest)
	measureStarted, err = servod.FindSubtestStartTime(subtestDir)
	if err != nil {
		s.Fatal("Failed to get subtest start time: ", err)
	}
	lastTimelineValue, err := servod.FindSubtestLastTimelineValue(subtestDir)
	if err != nil {
		s.Fatal("Failed to get subtest last timeline value: ", err)
	}

	measureEnded := measureStarted + lastTimelineValue

	// Perf maps metrics to value but they modify key before saving, so dig up metric again.
	for metric := range servoResult.GetValues() {
		if metric.Name == intervalMetricName {
			intervalMetric = metric
			break
		}
	}

	intervalData := servoResult.GetValues()[intervalMetric]
	overlapStartIdx := 0
	overlapEndIdx := len(intervalData)
	for index, value := range intervalData {
		if value >= measureStarted {
			overlapStartIdx = index
			break
		}
	}
	for index, value := range intervalData {
		if value > measureEnded {
			overlapEndIdx = index
			break
		}
	}

	if overlapStartIdx == overlapEndIdx {
		s.Fatal("No data from Servo")
	}

	// Trim start and end values to subtest timing only.
	for key, values := range servoResult.GetValues() {
		servoResult.GetValues()[key] = values[overlapStartIdx:overlapEndIdx]
	}

	intervalData = servoResult.GetValues()[intervalMetric]
	for i, val := range intervalData {
		intervalData[i] = val - measureStarted
	}

	s.Logf("%d sample collected over %f secs", len(intervalData), intervalData[len(intervalData)-1]-intervalData[0])
	for metric, values := range servoResult.GetValues() {
		if metric.Name != intervalMetricName {
			sum := 0.
			for _, v := range values {
				sum += v
			}
			s.Logf("Average power measured by %s: %f W", metric.Name, sum/float64(len(values)))
		}
	}

	if err := servoResult.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}

}

func chargeBattery(ctx context.Context, s *testing.State) {
	s.Logf("Waiting for battery to reach %f", chargeTarget)

	// TODO: b/301489823 - Use dump_power_status.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := s.DUT().Conn().CommandContext(ctx, "power_supply_info").Output()
		if err != nil {
			return errors.Wrap(err, "failed to get power_supply_info")
		}
		if regexp.MustCompile(`.*online:\s*yes`).Find(out) == nil {
			return testing.PollBreak(errors.Wrap(err, "power is not connected"))
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
