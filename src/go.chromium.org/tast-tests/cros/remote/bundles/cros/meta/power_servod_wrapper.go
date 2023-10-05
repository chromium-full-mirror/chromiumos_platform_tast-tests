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

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerServodWrapper,
		Desc:         "Runs test while capturing power data using servod",
		LacrosStatus: testing.LacrosVariantUnneeded,
		BugComponent: "b:167191", // ChromeOS > Platform > System > Power
		Contacts:     []string{"cros-pe-pnp@google.com", "khwon@google.com"},
		Timeout:      24 * time.Hour, // Depends on subtest, so set maximum value here.
		Params: []testing.Param{
			// Special test cases can be added as a Param here
			{
				Name: "bare",
				Val:  "",
			},
			{
				Name:      "cpd_vp_h264_1080_30fps",
				Val:       "power.VideoPlayback.h264_1080_30fps_ash",
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
			{
				Name:      "cpd_vp_vp9_1080_30fps",
				Val:       "power.VideoPlayback.vp9_1080_30fps_ash",
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
		},
		Vars: []string{"servo", "test_to_run"},
	})
}

const (
	servoPowerMeasureInterval = 2 * time.Second
	chargeTarget              = 75.
	intervalMetricName        = "t"
)

func PowerServodWrapper(ctx context.Context, s *testing.State) {
	subtest := s.Param().(string)
	resultsDir := filepath.Join(s.OutDir(), "subtest_results")

	// servoCtx is used for async function measuring power.
	servoCtx, servoCancel := context.WithCancel(ctx)

	cleanupCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if varTest, ok := s.Var("test_to_run"); ok {
		subtest = varTest
	}
	s.Log("Test to run: ")
	s.Log(subtest)

	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctx)

	chargeBattery(cleanupCtx, s)

	// Disable charging
	if _, err := s.DUT().Conn().CommandContext(cleanupCtx, "ectool", "chargeoverride", "dontcharge").Output(); err != nil {
		s.Fatal("Unable to disable charging: ", err)
	}

	cpdVBATMetric := perf.Metric{
		Name:      "CPD_VBAT",
		Unit:      "W",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalMetricName,
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
		if err := pxy.Servo().SetInt(ctx, "ft4232h_generic.CPD_VBAT_acc_clear", 1); err != nil {
			s.Fatal("Failed to clear accumulator: ", err)
		}
		pv := perf.NewValues()
		for {
			select {
			case <-time.After(servoPowerMeasureInterval):
				mw, err := pxy.Servo().GetFloat(ctx, "ft4232h_generic.CPD_VBAT_avg_mw")
				if err != nil {
					s.Fatal("Failed to get mw from servo instance: ", err)
				}
				// Clear the accumulator at the end of the loop, so that during
				// the interval we're accumulating.
				if err := pxy.Servo().SetInt(ctx, "ft4232h_generic.CPD_VBAT_acc_clear", 1); err != nil {
					s.Fatal("Failed to clear accumulator: ", err)
				}
				pv.Append(cpdVBATMetric, mw/1000.)
				pv.Append(intervalMetric, float64(time.Now().Unix()))
			case <-servoCtx.Done():
				ch <- pv
				return
			}
		}
	}()

	skippedTests := tastrun.RunAndEvaluate(cleanupCtx, s, []string{}, []string{subtest}, resultsDir, tastrun.SkipPolicyDisallowSkipping)
	if len(skippedTests) > 0 {
		s.Fatal("Test is skipped, abort post-processing")
	} else {
		s.Log("Finished test")
	}

	servoCancel()
	servoResult := <-ch

	// Enable charging
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
}

func chargeBattery(ctx context.Context, s *testing.State) {
	s.Logf("Waiting for battery to reach %f", chargeTarget)
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
