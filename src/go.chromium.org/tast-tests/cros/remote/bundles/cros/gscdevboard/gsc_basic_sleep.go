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

type testBasicSleepConfig struct {
	DeepSleep     bool
	Bus           ti50.TpmBus
	WakeSignal    ti50.GpioName
	WakeSignalVal bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCBasicSleep,
		Desc:    "Verifies GSC can enter sleep",
		Timeout: 2 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Firmware Developers
			"mruthven@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "deep_sleep",
			Val: testBasicSleepConfig{
				DeepSleep:     true,
				Bus:           ti50.TpmBusSpi, // The TPM type doesn't matter for deep sleep.
				WakeSignal:    ti50.GpioTi50PltRstL,
				WakeSignalVal: true,
			},
		}, {
			Name: "regular_spi",
			Val: testBasicSleepConfig{
				DeepSleep:     false,
				Bus:           ti50.TpmBusSpi,
				WakeSignal:    ti50.GpioTi50PltRstL,
				WakeSignalVal: false,
			},
		}, {
			Name: "regular_i2c",
			Val: testBasicSleepConfig{
				DeepSleep:     false,
				Bus:           ti50.TpmBusI2c,
				WakeSignal:    ti50.GpioTi50PltRstL,
				WakeSignalVal: false,
			},
		}},
	})
}

// GSCBasicSleep verifies GSC can enter and resume from regular and normal sleep.
func GSCBasicSleep(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	testConfig := s.Param().(testBasicSleepConfig)

	b.ResetAndTpmStartupForBus(ctx, i, testConfig.Bus, ti50.CcdDisconnected, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	var sleepDelay time.Duration
	switch b.TestbedType {
	case ti50.GscH1Shield:
		// H1 takes 20s to enter sleep. Add 5s to be safe.
		sleepDelay = 25 * time.Second
	case ti50.GscDTAndreiboard, ti50.GscDTShield:
		// DT takes 60s to enter sleep. Add 5s to be safe.
		sleepDelay = 65 * time.Second
	default:
		s.Fatalf("Unknown testbed type: %s", b.TestbedType)
	}

	err := b.WaitForPowerRise(ctx, 10.0, 5*time.Second)
	if err != nil {
		s.Fatalf("Did not hit the AP on power threshold at the start of the test: %s", err)
	}

	c := b.ReadGscTotalMilliAmps(ctx)
	s.Logf("mA start: %f", c)

	waitForSleep := b.WaitUntilNormalSleep
	if testConfig.DeepSleep {
		// Turn off the AP to enter deep sleep.
		b.GpioSet(ctx, testConfig.WakeSignal, !testConfig.WakeSignalVal)
		waitForSleep = b.WaitUntilDeepSleep
	}
	err = waitForSleep(ctx, i, sleepDelay)
	if err != nil {
		// Run some commands that will help debug sleep issues.
		i.Command(ctx, "sleepmask")
		i.Command(ctx, "gpioget")
		s.Fatalf("Did not enter sleep with %s asserted: %s", testConfig.WakeSignal, err)
	}
	// Verify GSC entered sleep.
	c = b.ReadGscTotalMilliAmps(ctx)
	s.Logf("mA in sleep: %f", c)

	b.GpioSet(ctx, testConfig.WakeSignal, testConfig.WakeSignalVal)
	b.GpioSet(ctx, testConfig.WakeSignal, !testConfig.WakeSignalVal)

	err = b.WaitForPowerRise(ctx, 10.0, 1*time.Second)
	if err != nil {
		s.Fatalf("Did not hit the AP on power threshold at the end of the test: %s", err)
	}
	c = b.ReadGscTotalMilliAmps(ctx)
	s.Logf("mA after AP_ON: %f", c)
}
