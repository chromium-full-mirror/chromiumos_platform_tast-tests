// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

var testDrift time.Duration

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCTime,
		Desc:    "Check times reported by GSC",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

type testTimes struct {
	coldReset time.Time
	deepSleep time.Time
}

func testTimesNow() testTimes {
	now := time.Now()
	return testTimes{now, now}
}

func GSCTime(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	// H1 rounds to the nearest second. Allow for 2s of drift.
	if b.TestbedType == ti50.GscH1Shield {
		testDrift = 2 * time.Second
	} else {
		testDrift = 1 * time.Second
	}
	want := testTimesNow()
	b.ResetWithStraps(ctx, ti50.FfClamshell, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	checkTimes(ctx, s, i, want, "start")

	want = testTimesNow()
	i.Command(ctx, "reboot")
	if b.TestbedType != ti50.GscH1Shield {
		th.MustSucceed(i.WaitUntilRoBoot(ctx, time.Second), "console reboot")
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	checkTimes(ctx, s, i, want, "reboot 1")

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioApplyStrap(ctx, ti50.CCDModeOff)
	s.Log("Waiting for deep sleep")
	th.MustSucceed(b.WaitUntilDeepSleep(ctx, i, ti50.WaitForSleepTimeout), "Sleep when AP off")
	want.deepSleep = time.Now()
	b.GpioApplyStrap(ctx, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 is awake")
	checkTimes(ctx, s, i, want, "deep sleep")

	want = testTimesNow()
	i.Command(ctx, "reboot")
	if b.TestbedType != ti50.GscH1Shield {
		th.MustSucceed(i.WaitUntilRoBoot(ctx, time.Second), "console reboot")
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	checkTimes(ctx, s, i, want, "reboot 2")

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.GpioApplyStrap(ctx, ti50.CCDModeOff)
	s.Log("Waiting for normal sleep")
	th.MustSucceed(b.WaitUntilNormalSleep(ctx, i, ti50.WaitForSleepTimeout), "Sleep when AP on")
	b.GpioApplyStrap(ctx, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 is awake")
	checkTimes(ctx, s, i, want, "normal sleep")

	if b.TestbedType == ti50.GscH1Shield {
		return
	}
	// Push all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, false)
	b.GpioSet(ctx, ti50.GpioTi50KsiBack, false)
	s.Log("Waiting for keycombo reset")
	th.MustSucceed(i.WaitUntilRoBoot(ctx, 15*time.Second), "reset key combo")
	want = testTimesNow()
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	// Release all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiBack, true)
	checkTimes(ctx, s, i, want, "reboot 3")
}

func checkTimes(ctx context.Context, s *testing.State, i *ti50.CrOSImage, want testTimes, label string) {
	s.Logf("Checking %s", label)
	testing.Sleep(ctx, time.Second*10) // GoBigSleepLint: Delay to test timers.
	now := time.Now()
	got, err := i.Time(ctx)
	if err != nil {
		s.Fatal("gettime failed: ", err)
	}
	checkTime(ctx, s, got.ColdResetTime, now.Sub(want.coldReset), label+" cold reset")
	checkTime(ctx, s, got.DeepSleepTime, now.Sub(want.deepSleep), label+" deep sleep")
}

func checkTime(ctx context.Context, s *testing.State, got, want time.Duration, label string) {
	want = want.Round(time.Millisecond)
	err := want - got
	msg := fmt.Sprintf("%s: got %s, want %s+-%s, err %s", label, got, want, testDrift, err)
	s.Log(msg)
	if err.Abs() > testDrift {
		s.Error(msg)
	}
}
