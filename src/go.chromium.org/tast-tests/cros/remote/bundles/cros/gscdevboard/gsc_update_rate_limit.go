// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

const (
	reboot  = "reboot"
	powerOn = "power-on"

	rebootFlag  = ti50.GscResetFlagHard
	powerOnFlag = ti50.GscResetFlagPowerOn

	updateDelay = time.Second * 61
)

type updateConfig struct {
	resetType    string
	limitsUpdate bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCUpdateRateLimit,
		Desc:    "Verify GSC rate limits updates correctly after different types of resets",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCUpdate,
		Params: []testing.Param{{
			Name: "reboot",
			Val: updateConfig{
				resetType:    reboot,
				limitsUpdate: true,
			},
		}, {
			Name: "poweron",
			Val: updateConfig{
				resetType:    powerOn,
				limitsUpdate: false,
			},
		}},
	})
}

func reset(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, resetType string) {

	var resetFlag uint32
	switch resetType {
	case reboot:
		s.Log("Running reboot")
		i.SendConsoleRebootCmd(ctx)
		resetFlag = rebootFlag
	case powerOn:
		s.Log("Running power-on reset")
		b.GpioSet(ctx, ti50.GpioTi50ResetL, false)
		b.GpioSet(ctx, ti50.GpioTi50ResetL, true)
		resetFlag = powerOnFlag
	default:
		s.Fatalf("Invalid reset type: %s", resetType)
	}

	err := i.WaitUntilBooted(ctx)
	if err != nil {
		s.Fatalf("GSC failed to boot after %s reset: %s", resetType, err)
	}

	sysinfo, err := i.Sysinfo(ctx)
	if err != nil {
		s.Fatalf("Unable to run sysinfo: %s", err)
	}
	if sysinfo.ResetFlags&resetFlag != resetFlag {
		s.Fatalf("%s reset: %x not found in %x", resetType, resetFlag, sysinfo.ResetFlags)
	}
	s.Logf("Ran %s reset", resetType)
}

// GSCUpdateRateLimit requires HW setup with SuzyQ cable from devboard to drone/workstation.
func GSCUpdateRateLimit(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	testConfig := s.Param().(updateConfig)
	resetType := testConfig.resetType
	limitsUpdate := testConfig.limitsUpdate

	// Valid image under test path.
	if f.ImagePath == "" {
		s.Fatal("Supply buildurl")
	}

	imageUnderTest := f.ImagePath
	_, releaseVer, _, _, err := b.GSCToolBinVersion(ctx, imageUnderTest)
	th.MustSucceed(err, "failed to parse image under test version")

	s.Logf("Image under test %s: %s", releaseVer, imageUnderTest)

	// Connect Suzyq, so the test can update over ccd.
	s.Log("Simulating insertion of SuzyQ and resetting")
	b.ResetWithStraps(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	reset(ctx, s, b, i, resetType)
	b.WaitUntilCCDConnected(ctx)

	gscTime, err := i.Time(ctx)
	th.MustSucceed(err, "failed to run gettime")
	s.Logf("GSC time: %+v", gscTime)
	err = b.UpdateOnce(ctx, i, imageUnderTest, releaseVer)
	if limitsUpdate {
		if err == nil {
			s.Fatalf("Did not block update after %s reset Time(%+v)", resetType, gscTime)
		}
		s.Logf("Blocked update after %s reset", resetType)
	} else if err != nil {
		s.Fatalf("Blocked update after %s reset Time(%+v)", resetType, gscTime)
	} else {
		s.Logf("Ran update after %s reset", resetType)
	}

	reset(ctx, s, b, i, resetType)

	// GoBigSleepLint sleeping for known required period of time.
	testing.Sleep(ctx, updateDelay)

	gscTime, err = i.Time(ctx)
	th.MustSucceed(err, "failed to run gettime")

	err = b.UpdateOnce(ctx, i, imageUnderTest, releaseVer)
	if err != nil {
		s.Fatalf("Failed to run update after %s reset Time(%+v)", resetType, gscTime)
	}
	s.Logf("Successfully ran the %s update rate limit test", resetType)
}
