// Copyright 2025 The ChromiumOS Authors
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
	// GSC opens ccd after 5 power button presses with short unlock
	expectedShortPressCount = 5
	// Try up to 10 power button presses
	maxTestPressCount = 10
)

type configOpenAPOff struct {
	strap       ti50.GpioStrap
	resetType   uint32
	resetSignal uint64
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCOpenAPOff,
		Desc:    "Verify GSC can open ccd before the AP turns on",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "plt_rst_power_on",
			Val: configOpenAPOff{
				strap:       ti50.TpmSpi,
				resetType:   ti50.GscResetFlagPowerOn,
				resetSignal: ti50.BoardPropPltRst,
			},
			ExtraAttr: []string{"gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310"},
		}, {
			Name: "plt_rst_hard",
			Val: configOpenAPOff{
				strap:       ti50.TpmSpi,
				resetType:   ti50.GscResetFlagHard,
				resetSignal: ti50.BoardPropPltRst,
			},
			ExtraAttr: []string{"gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310"},
		}, {
			Name: "plt_rst_deep_sleep",
			Val: configOpenAPOff{
				strap:       ti50.TpmSpi,
				resetType:   ti50.GscResetFlagHibernate,
				resetSignal: ti50.BoardPropPltRst,
			},
			ExtraAttr: []string{"gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310"},
		}, {
			Name: "sys_rst_power_on",
			Val: configOpenAPOff{
				strap:       ti50.TpmSpiSysRst,
				resetType:   ti50.GscResetFlagPowerOn,
				resetSignal: ti50.BoardPropSysRst,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name: "sys_rst_hard",
			Val: configOpenAPOff{
				strap:       ti50.TpmSpiSysRst,
				resetType:   ti50.GscResetFlagHard,
				resetSignal: ti50.BoardPropSysRst,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name: "sys_rst_deep_sleep",
			Val: configOpenAPOff{
				strap:       ti50.TpmSpiSysRst,
				resetType:   ti50.GscResetFlagHibernate,
				resetSignal: ti50.BoardPropSysRst,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}},
	})
}

// GSCOpenAPOff verifies GSC can open ccd before the AP turns on
func GSCOpenAPOff(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	config := s.Param().(configOpenAPOff)

	s.Logf("Restarting GSC with %s", config.strap)
	b.ResetWithStraps(ctx, config.strap, ti50.ApOff, ti50.CcdDisconnected)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	brdprop, err := i.BoardProperties(ctx)
	th.MustSucceed(err, "failed to get GSC brdprop")
	s.Logf("brdprop: %x", brdprop)
	if brdprop&config.resetSignal != config.resetSignal {
		s.Fatalf("%x not found in brdprops %x", config.resetSignal, brdprop)
	}
	s.Log("setup straps")

	err = i.TestlabOpen(ctx)
	th.MustSucceed(err, "failed to open ccd")
	err = i.SetCCDCapability(ctx, ti50.BatteryBypassPP, ti50.CapIfOpened)
	th.MustSucceed(err, "failed to set BatteryBypassPP")
	err = i.SetCCDCapability(ctx, ti50.OpenNoTPMWipe, ti50.CapIfOpened)
	th.MustSucceed(err, "failed to set OpenNoTPMWipe")
	err = i.SetCCDCapability(ctx, ti50.UnlockNoReboot, ti50.CapIfOpened)
	th.MustSucceed(err, "failed to set UnlockNoReboot")
	err = i.SetCCDCapability(ctx, ti50.UnlockNoShortPP, ti50.CapIfOpened)
	th.MustSucceed(err, "failed to set UnlockNoShortPP")
	err = i.CCDLock(ctx)
	th.MustSucceed(err, "failed to lock ccd")
	ccd, err := i.Command(ctx, "ccd")
	th.MustSucceed(err, "failed to get ccd output")
	s.Logf("ccd: %s", ccd)

	switch config.resetType {
	case ti50.GscResetFlagPowerOn:
		b.ResetWithStraps(ctx, config.strap, ti50.ApOff)
	case ti50.GscResetFlagHard:
		err = i.Reboot(ctx)
		th.MustSucceed(err, "reboot failed")
	case ti50.GscResetFlagHibernate:
		// Get some state for debugging
		gpioget, err := i.Command(ctx, "gpioget")
		th.MustSucceed(err, "failed to get gpioget output")
		s.Logf("gpioget: %s", gpioget)
		sleepmask, err := i.Command(ctx, "sleepmask")
		th.MustSucceed(err, "failed to get sleepmask output")
		s.Logf("sleepmask: %s", sleepmask)

		err = b.WaitUntilDeepSleep(ctx, i, ti50.WaitForSleepTimeout)
		th.MustSucceed(err, "failed to enter deep sleep")
		// Wake GSC with the power button
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
		testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Simulating button press
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	default:
		s.Fatalf("Unsupported reset type: %x", config.resetType)
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	brdprop, err = i.BoardProperties(ctx)
	th.MustSucceed(err, "failed to get GSC brdprop")
	s.Logf("brdprop: %x", brdprop)
	if brdprop&config.resetSignal != config.resetSignal {
		s.Fatalf("%x not found in brdprops %x after reset", config.resetSignal, brdprop)
	}
	s.Log("straps valid after reset")

	// Verify GSC did the correct reset
	sysinfo, err := i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to get GSC sysinfo")
	s.Logf("sysinfo reset flags: %x", sysinfo.ResetFlags)
	if sysinfo.ResetFlags&config.resetType != config.resetType {
		s.Fatalf("%x not found in sysinfo reset flags %x", config.resetType, sysinfo.ResetFlags)
	}
	s.Log("Ran GSC reset")

	out, err := i.Command(ctx, "ccd open")
	th.MustSucceed(err, "failed to run ccd open")
	s.Logf("CCD open output: %s", out)
	var state ti50.CCDLevel
	var j int
	for j = 0; j < maxTestPressCount; j++ {
		state, err = i.CCDLevel(ctx)
		th.MustSucceed(err, "failed to get ccd level")
		s.Logf("CCD Level: %s", state)
		if state == ti50.Open {
			break
		}
		testing.Sleep(ctx, time.Second) // GoBigSleepLint: delay between power button presses
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
		testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Simulating button press
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
		s.Log("pressed power button")
		// Log the console output after the power button press. This
		// will include the ccd open messages if it was the last power
		// button press required to open ccd.
		if out, err := i.Command(ctx, ""); err == nil {
			s.Log("out: ", out)
		}
	}
	if j != expectedShortPressCount {
		s.Errorf("Unexpected power button count: expected %d got %d", expectedShortPressCount, j)
	}
	if state != ti50.Open {
		s.Error("Unable to open ccd")
	}
	s.Logf("Opened ccd after %d power button presses", j)
}
