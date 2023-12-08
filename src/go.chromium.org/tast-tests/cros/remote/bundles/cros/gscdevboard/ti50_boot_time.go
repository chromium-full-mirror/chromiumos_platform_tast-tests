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
			"chromeos-faft@google.com",
			"ti50-core@google.com",
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

var (
	bootTraceRe       = regexp.MustCompile(`([^ ]+):\s*(\d+) ms`)
	coldResetStagesRe = regexp.MustCompile(`ProjectStart,EcRstAsserted,(EcRstAsserted,)?Tp?mRstDeasserted,EcRstDeasserted,TpmAppReady`)
	deepSleepStagesRe = regexp.MustCompile(`ProjectStart,Tp?mRstDeasserted,EcRstDeasserted,TpmAppReady`)
)

// Ti50BootTime measures boot time.
func Ti50BootTime(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)
	pv := perf.NewValues()

	// Cold reboot with AP on.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	b.WaitUntilCCDConnected(ctx)

	checkBootTrace(ctx, s, b, pv, "ColdReset_", coldResetStagesRe)

	// Wake from deep sleep by AP on.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	s.Log("Waiting for deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, 70*time.Second), "deep sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	b.WaitUntilCCDConnected(ctx)

	checkBootTrace(ctx, s, b, pv, "DeepSleep_", deepSleepStagesRe)

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf data: ", err)
	}

}

func checkBootTrace(ctx context.Context, s *testing.State, b utils.DevboardHelper, pv *perf.Values, prefix string, expected *regexp.Regexp) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	logTime := func(label string, t uint32) {
		pv.Set(perf.Metric{
			Name:      label,
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, float64(t))
		s.Logf("%s: %d ms", label, t)
	}

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
		totalTime += v
		if t[1] == "ProjectStart" || t[1] == "EcRstDeasserted" || t[1] == "TpmAppReady" {
			logTime(prefix+t[1], uint32(totalTime))
		}
	}
	allstages := strings.Join(stages, ",")
	if !expected.MatchString(allstages) {
		s.Errorf("Expected %q, got %q", expected, allstages)
	}
}
