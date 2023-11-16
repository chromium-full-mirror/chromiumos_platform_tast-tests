// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type cCDCapabilitiesRebootECAP struct {
	capState         ti50.CCDCapState
	expectGPIOToggle bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    CCDCapabilitiesRebootECAP,
		Desc:    "Test RebootECAP CCD capability by monitoring GPIOs",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
		Params: []testing.Param{{
			Name: "cap_default",
			Val: cCDCapabilitiesRebootECAP{
				capState:         ti50.CapDefault,
				expectGPIOToggle: false,
			},
		}, {
			Name: "cap_always",
			Val: cCDCapabilitiesRebootECAP{
				capState:         ti50.CapAlways,
				expectGPIOToggle: true,
			},
		}, {
			Name: "cap_if_opened",
			Val: cCDCapabilitiesRebootECAP{
				capState:         ti50.CapIfOpened,
				expectGPIOToggle: false,
			},
		}},
		// TODO(mvertescher): Add `cap_unless_locked` test for Cr50 only
	})
}

func CCDCapabilitiesRebootECAP(ctx context.Context, s *testing.State) {
	userParams := s.Param().(cCDCapabilitiesRebootECAP)
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("Resetting GSC and starting up")
	_ = b.ResetAndTpmStartup(ctx, i, ti50.TpmBusSpi, ti50.CcdSuzyQ, ti50.FfClamshell)

	if err := i.CCDOpen(ctx); err != nil {
		s.Fatal("Failed to open CCD to test RebootECAP cap: ", err)
	}

	if err := i.SetCCDCapability(ctx, ti50.RebootECAP, userParams.capState); err != nil {
		s.Fatal("Failed to set RebootECAP cap: ", err)
	}

	verifyResetSignalTestsPass(ctx, s, b, i)

	if err := i.CCDLock(ctx); err != nil {
		s.Fatal("Failed to lock CCD to test RebootECAP cap: ", err)
	}

	if userParams.expectGPIOToggle {
		verifyResetSignalTestsPass(ctx, s, b, i)
	} else {
		// `ecrst pulse` is a safe command so this should still succeed
		verifyResetSignal(ctx, s, b, i, ti50.GpioTi50EcRstL, ecrstPulse, true)
		// The following operations should fail
		verifyResetSignal(ctx, s, b, i, ti50.GpioTi50EcRstL, ecrstOnOff, false)
		verifyResetSignal(ctx, s, b, i, ti50.GpioTi50SysRstL, sysrstPulse, false)
		verifyResetSignal(ctx, s, b, i, ti50.GpioTi50SysRstL, sysrstOnOff, false)
	}
}

func verifyResetSignalTestsPass(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage) {
	verifyResetSignal(ctx, s, b, i, ti50.GpioTi50EcRstL, ecrstPulse, true)
	verifyResetSignal(ctx, s, b, i, ti50.GpioTi50EcRstL, ecrstOnOff, true)
	verifyResetSignal(ctx, s, b, i, ti50.GpioTi50SysRstL, sysrstPulse, true)
	verifyResetSignal(ctx, s, b, i, ti50.GpioTi50SysRstL, sysrstOnOff, true)
}

// verifyResetSignal uses the GPIO monitor to test that either the reset signal
// `gpio` (AP or EC) is properly asserted or not based on `expectSignalToggled`
// when the `resetFn` is executed.
func verifyResetSignal(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, gpio ti50.GpioName, resetFn func(context.Context, *ti50.CrOSImage, bool) error, expectSignalToggled bool) {
	gpioMonitor := b.GpioMonitorStart(ctx, gpio)
	if err := resetFn(ctx, i, expectSignalToggled); err != nil {
		s.Fatalf("Failed execute the signal %s reset operation: %s", gpio, err)
	}
	testing.Sleep(ctx, time.Second) // GoBigSleepLint: No good way to poll for EC_RST, SYS_RST
	events := b.GpioMonitorFinish(ctx, gpioMonitor)

	if expectSignalToggled {
		reset := events.FindFirst(gpio, utils.GpioEdgeFalling)
		if reset == nil {
			s.Errorf("%s did not reset", gpio)
			return
		}
		released := events.FindFirstAfter(*reset, gpio)
		if released == nil {
			s.Errorf("%s did not release", gpio)
			return
		}
	} else {
		if events.FindFirst(gpio, utils.GpioEdgeFalling) != nil {
			s.Errorf("Found a %s GPIO reset event when none were expected", gpio)
		}
	}
}

func ecrstOnOff(ctx context.Context, i *ti50.CrOSImage, expectCommandsToPass bool) error {
	err1 := i.EcrstOn(ctx)
	err2 := i.EcrstOff(ctx)

	return checkTwoCommandErrors(err1, err2, expectCommandsToPass)
}

func ecrstPulse(ctx context.Context, i *ti50.CrOSImage, expectCommandsToPass bool) error {
	return checkCommandError(i.EcrstPulse(ctx), expectCommandsToPass)
}

func sysrstOnOff(ctx context.Context, i *ti50.CrOSImage, expectCommandsToPass bool) error {
	err1 := i.SysrstOn(ctx)
	err2 := i.SysrstOff(ctx)

	return checkTwoCommandErrors(err1, err2, expectCommandsToPass)
}

func sysrstPulse(ctx context.Context, i *ti50.CrOSImage, expectCommandsToPass bool) error {
	return checkCommandError(i.SysrstPulse(ctx), expectCommandsToPass)
}

func checkCommandError(err error, expectCommandsToPass bool) error {
	if expectCommandsToPass && err != nil {
		return errors.Wrap(err, "expected command to pass, but it failed")
	} else if !expectCommandsToPass && err == nil {
		return errors.New("expected command to fail, but it passed")
	}
	return nil
}

func checkTwoCommandErrors(err1, err2 error, expectCommandsToPass bool) error {
	if expectCommandsToPass {
		if err1 != nil || err2 != nil {
			return errors.New("expected commands to pass, but at least one failed")
		}
	} else if !expectCommandsToPass {
		if err1 == nil || err2 == nil {
			return errors.New("expected commands to fail, but at least one passed")
		}
	}
	return nil
}
