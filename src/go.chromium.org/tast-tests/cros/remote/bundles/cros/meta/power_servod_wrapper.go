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

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	ps "go.chromium.org/tast-tests/cros/common/power/powerpb"
	"go.chromium.org/tast-tests/cros/common/servo"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/tastrun"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

/**
Example:
tast run -var "subtest=power.ExampleUI.ash_kbbl" $DUT_IP meta.PowerServodWrapper.cpd_manual

To specify a servo port, use: -var "servo=<servo-host>:<servo-port>"

For more options see go/tast-running#Running-tests-with-Servo
*/

type testParams struct {
	// cpd indicates if the servo used is CPD (special formatting)
	cpd bool
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
		Contacts:     []string{"cros-pe-pnp@google.com", "khwon@google.com", "zhaon@google.com"},
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
					cpd: true,
				},
			},
			{
				Name: "cpd_vp_h264_1080_30fps",
				Val: testParams{
					cpd:     true,
					subtest: "power.VideoPlayback.h264_1080_30fps_ash",
				},
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
			{
				Name: "cpd_vp_vp9_1080_30fps",
				Val: testParams{
					cpd:     true,
					subtest: "power.VideoPlayback.vp9_1080_30fps_ash",
				},
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
			{
				Name: "cpd_vc_25m",
				Val: testParams{
					cpd:     true,
					subtest: "power.VideoCall.25m_ash",
				},
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
			{
				Name: "cpd_browsing",
				Val: testParams{
					cpd:     true,
					subtest: "power.Browsing.ash",
				},
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
			{
				Name: "cpd_browsing_heavy",
				Val: testParams{
					cpd:     true,
					subtest: "power.Browsing.heavy_ash",
				},
				ExtraAttr: []string{"group:power", "power_cpd"},
			},
		},
		Vars:        []string{"servo", "subtest"},
		ServiceDeps: []string{"tast.common.power.powerpb.LocalInfoService"},
	})
}

const (
	// defaultServoPowerMeasureInterval in seconds.
	// Note: Do not set the interval too low as this will cause the snapshots to fail.
	defaultServoPowerMeasureInterval = "4"
	chargeTarget                     = 75.
)

func PowerServodWrapper(ctx context.Context, s *testing.State) {
	// servoCtx is used for async function measuring power.
	servoCtx, servoCancel := context.WithCancel(ctx)
	defer servoCancel()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	// commandCtx is used for running subtest.
	commandCtx, commandCancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer commandCancel()

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

	// Handle filters.
	var filters []*regexp.Regexp
	filters = make([]*regexp.Regexp, 0)
	if param.filter != "" {
		filters = append(filters, regexp.MustCompile(param.filter))
	}

	chargeBattery(ctx, s)

	// Disable charging.
	if _, err := s.DUT().Conn().CommandContext(ctx, "ectool", "chargeoverride", "dontcharge").Output(); err != nil {
		s.Fatal("Unable to disable charging: ", err)
	}

	defer func() {
		// Enable charging.
		// TODO: b/303548068 - Sync CC with setup_battery.go.
		if _, err := s.DUT().Conn().CommandContext(cleanupCtx, "ectool", "chargeoverride", "off").Output(); err != nil {
			s.Fatal(cleanupCtx, "Unable to enable charging: ", err)
		}
	}()

	sm, err := cp.NewServodMetrics(servoCtx, pxy.Servo(), param.cpd, filters...)
	if err != nil {
		s.Fatal("Failed to setup servod metrics: ", err)
	}
	servoTimeline, err := perf.NewTimeline(servoCtx, []perf.TimelineDatasource{sm}, perf.Interval(servoPowerMeasureInterval))
	if err != nil {
		s.Fatal("Failed to build metrics timeline: ", err)
	}
	if err := servoTimeline.Start(servoCtx); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}
	if err := servoTimeline.StartRecording(servoCtx); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	s.Log("Starting subtest: ", subtest)
	resultsDir := filepath.Join(s.OutDir(), "subtest_results")
	skippedTests := tastrun.RunAndEvaluate(commandCtx, s, []string{}, []string{subtest}, resultsDir, tastrun.SkipPolicyDisallowSkipping)
	if len(skippedTests) > 0 {
		s.Fatal("Test is skipped, abort post-processing")
	}

	// The subtest has finished, so cancel the servoCtx.
	servoCancel()
	select {
	case <-commandCtx.Done():
		s.Fatal("Subtest is cancelled")
	default:
		// Not canceled in servo goroutine.
		s.Log("Finished subtest: ", subtest)
	}

	servoResult, err := servoTimeline.StopRecording(servoCtx)
	if err != nil {
		s.Fatal("Failed to stop recording: ", err)
	}

	mS, mE, err := cp.FindMeasureStartAndEnd(ctx, s.OutDir(), subtest)
	if err != nil {
		s.Fatal("Failed to find start and end of subtest: ", err)
	}
	servoResult, err = cp.TrimSubtestResults(mS, mE, servoResult)
	if err != nil {
		s.Fatal("Failed to format subtest results: ", err)
	}
	if err := servoResult.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf data for crosbolt: ", err)
	}

	devInfo, oneTimeMetrics, err := localDUTInfo(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to get local DUT info: ", err)
	}

	if _, err := cp.CreateSaveUploadPowerLog(ctx, s.OutDir(), subtest, "", servoResult, devInfo, oneTimeMetrics); err != nil {
		s.Fatal("Failed to save and upload power log and perf: ", err)
	}

}

func localDUTInfo(ctx context.Context, dut *dut.DUT, rpchint *testing.RPCHint) (*ps.DeviceInfo, *ps.OneTimeMetrics, error) {
	cl, err := rpc.Dial(ctx, dut, rpchint)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	client := ps.NewLocalInfoServiceClient(cl.Conn)
	devInfo, err := client.GetDeviceInfoFromDUT(ctx, &empty.Empty{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to call gRPC service for device info")
	}

	oneTimeMetrics, err := client.GetOneTimeMetricsFromDUT(ctx, &empty.Empty{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to call gRPC service for one time metrics")
	}
	return devInfo, oneTimeMetrics, nil
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
