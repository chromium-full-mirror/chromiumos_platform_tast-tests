// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50BootTime,
		Desc:    "Measure boot time",
		Timeout: 2 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield", "gsc_ot_shield"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

var (
	bootTraceRe       = regexp.MustCompile(`(\S+):\s*(\d+) ms`)
	coldResetStagesRe = regexp.MustCompile(`ProjectStart,EcRstAsserted,(EcRstAsserted,)?Tp?mRstDeasserted,EcRstDeasserted,TpmAppReady`)
	deepSleepStagesRe = regexp.MustCompile(`ProjectStart,Tp?mRstDeasserted,EcRstDeasserted,TpmAppReady`)
	metricsRe         = regexp.MustCompile(`(\S+)_time:\s*(\d+)`)
)

// Ti50BootTime measures boot time.
func Ti50BootTime(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)
	pv := perf.NewValues()

	// Cold reboot with AP on.
	prefix := "ColdReset_"
	b.GpioSet(ctx, ti50.GpioTi50ResetL, false)
	var gpioMonitor utils.GpioMonitorSession
	if b.GscProperties().HasEcRstFet() {
		gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50ResetL, ti50.GpioTi50EcRstL, ti50.GpioTi50EcRstFet)
	} else {
		gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50ResetL, ti50.GpioTi50EcRstL)
	}
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.GpioSet(ctx, ti50.GpioTi50ResetL, true)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	checkGpioMonitor(ctx, s, b, pv, prefix, gpioMonitor, ti50.GpioTi50ResetL)
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	b.WaitUntilCCDConnected(ctx)
	checkBootTrace(ctx, s, b, pv, prefix, coldResetStagesRe)
	checkMetrics(ctx, s, b, pv, prefix)

	// Wake from deep sleep by AP on. Assert EC reset before sleep so we can
	// see a rising edge after wake.
	prefix = "DeepSleep_"
	th.MustSucceed(i.EcrstOn(ctx), "ecrst")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	s.Log("Waiting for deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, ti50.WaitForSleepTimeout), "deep sleep")
	if b.GscProperties().HasEcRstFet() {
		gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50PltRstL, ti50.GpioTi50EcRstL, ti50.GpioTi50EcRstFet)
	} else {
		gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50PltRstL, ti50.GpioTi50EcRstL)
	}
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	checkGpioMonitor(ctx, s, b, pv, prefix, gpioMonitor, ti50.GpioTi50PltRstL)
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	b.WaitUntilCCDConnected(ctx)
	checkBootTrace(ctx, s, b, pv, prefix, deepSleepStagesRe)
	checkMetrics(ctx, s, b, pv, prefix)

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf data: ", err)
	}

}

func logTime(s *testing.State, pv *perf.Values, label string, t uint32) {
	pv.Set(perf.Metric{
		Name:      label,
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, float64(t))
	s.Logf("%s: %d ms", label, t)
}

func checkGpioMonitor(ctx context.Context, s *testing.State, b utils.DevboardHelper, pv *perf.Values, prefix string, gpioMonitor utils.GpioMonitorSession, triggerGpio ti50.GpioName) {
	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.VLog(events)
	// Measure time from the rising edge of the triggerGpio.
	start := events.FindFirst(triggerGpio, utils.GpioEdgeRising)
	var end uint64
	if b.GscProperties().HasEcRstFet() {
		// The EC may be released by EcRstL multiple times (due to DT bug), so we want the last edge.
		// We also want the later of EcRstL rising and EcRstFet falling since both are necessary to
		// release the EC from reset.
		ecRstReleased := events.FindLast(ti50.GpioTi50EcRstL, utils.GpioEdgeRising)
		end = ecRstReleased.TimestampUS
		ecFetReleased := events.FindLast(ti50.GpioTi50EcRstFet, utils.GpioEdgeFalling)
		// When waking from deep sleep, EcRstFet remains low so there is no falling edge.
		if ecFetReleased != nil && ecFetReleased.TimestampUS > end {
			end = ecFetReleased.TimestampUS
		}
	} else {
		ecRstReleased := events.FindFirst(ti50.GpioTi50EcRstL, utils.GpioEdgeRising)
		end = ecRstReleased.TimestampUS
	}
	ecReleaseTime := (end - start.TimestampUS) / 1000
	logTime(s, pv, prefix+"EcRstGpioDeasserted", uint32(ecReleaseTime))
}

func checkBootTrace(ctx context.Context, s *testing.State, b utils.DevboardHelper, pv *perf.Values, prefix string, expected *regexp.Regexp) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	out, err := b.GSCToolCommand(ctx, "", "--boot_trace")
	th.MustSucceed(err, "read boot trace")
	s.VLogf(string(out))
	totalTime := 0
	times := bootTraceRe.FindAllStringSubmatch(string(out), -1)
	var stages []string
	for _, t := range times {
		stages = append(stages, t[1])
		v, err := strconv.Atoi(t[2])
		th.MustSucceed(err, "parse int")
		if prefix == "DeepSleep_" && t[1] == "ProjectStart" {
			// ProjectStart is not valid for deep sleep (b/318415821).
			continue
		}
		totalTime += v
		if t[1] == "ProjectStart" || t[1] == "EcRstDeasserted" || t[1] == "TpmAppReady" {
			logTime(s, pv, prefix+t[1], uint32(totalTime))
		}
	}
	allstages := strings.Join(stages, ",")
	if !expected.MatchString(allstages) {
		s.Errorf("Expected %q, got %q", expected, allstages)
	}
}

func checkMetrics(ctx context.Context, s *testing.State, b utils.DevboardHelper, pv *perf.Values, prefix string) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	out, err := b.GSCToolCommand(ctx, "", "--metrics", "--dauntless")
	th.MustSucceed(err, "read metrics")
	s.VLogf(string(out))
	times := metricsRe.FindAllStringSubmatch(string(out), -1)
	for _, t := range times {
		v, err := strconv.Atoi(t[2])
		th.MustSucceed(err, "parse int")
		logTime(s, pv, prefix+t[1]+"_duration", uint32(v))
	}
}
