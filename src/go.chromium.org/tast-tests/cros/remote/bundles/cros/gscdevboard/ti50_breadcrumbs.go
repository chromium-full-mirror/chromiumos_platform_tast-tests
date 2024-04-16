// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

// Breadcrumb event values from Ti50.
const (
	projectMain         = 1
	pmuPreInit          = 2
	projectStart        = 3
	projectRun          = 4
	pmuNormalSleep      = 5
	pmuDeepSleep        = 6
	pmuResumeFromSleep  = 7
	pmuTriggerSoftReset = 8
	pmuTriggerHardReset = 9
	nmi                 = 10
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Breadcrumbs,
		Desc:    "Check breadcrumbs storing PMU events over GSC reboot",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ti50-core@google.com",
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

func Ti50Breadcrumbs(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	var expected []int

	s.Log("Reset with clamshell straps and CCD connected")
	b.ResetWithStraps(ctx, ti50.FfClamshell, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	expected = append(expected, projectMain, pmuPreInit, projectStart, projectRun)
	checkBreadcrumbs(ctx, s, i, expected)

	s.Log("Disconnecting CCD to allow sleep")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	s.Log("Waiting for normal sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, 70*time.Second), "Sleep when AP on")
	s.Log("Waiting for deep sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, 70*time.Second), "Sleep when AP off")
	// Expect pmuResumeFromSleep only after pmuNormalSleep. Wake from pmuDeepSleep will follow
	// boot path instead.
	expected = append(expected, pmuNormalSleep, pmuResumeFromSleep, pmuDeepSleep)

	s.Log("Connecting CCD")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 is awake")
	expected = append(expected, projectMain, pmuPreInit, projectStart, projectRun)
	checkBreadcrumbs(ctx, s, i, expected)

	// Push all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, false)
	b.GpioSet(ctx, ti50.GpioTi50KsiBack, false)

	s.Log("Waiting for GSC to reset")
	th.MustSucceed(i.WaitUntilRoBoot(ctx, 15*time.Second), "reset key combo")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Release all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiBack, true)

	expected = append(expected, projectMain, pmuPreInit, projectStart, projectRun)
	checkBreadcrumbs(ctx, s, i, expected)
}

var breadcrumbsRegexp = regexp.MustCompile(`Breadcrumbs: *0x([0-9a-fA-F]+)`)

func checkBreadcrumbs(ctx context.Context, s *testing.State, i *ti50.CrOSImage, expected []int) {
	output, err := i.Command(ctx, "sysinfo")
	if err != nil {
		s.Fatal("sysinfo failed: ", err)
	}
	match := breadcrumbsRegexp.FindStringSubmatch(output)
	if match == nil {
		s.Fatal("sysinfo output: ", output)
	}
	s.Log("Breadcrumbs: 0x", match[1])
	// Decode from u64 into the list of breadcrumb event values. Each value is stored as 4 bits, so
	// one char of the hex string.
	var got []int
	for _, c := range match[1] {
		v, err := strconv.ParseInt(string(c), 16, 0)
		if err != nil {
			s.Fatal("parse int: ", match[1])
		}
		if v != 0 {
			got = append(got, int(v))
		}
	}
	if !reflect.DeepEqual(got, expected) {
		s.Fatalf("Expected %q, got %q", expected, got)
	}
}
