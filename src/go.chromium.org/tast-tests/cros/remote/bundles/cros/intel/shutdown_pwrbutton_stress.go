// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/remote/tabletmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShutdownPwrbuttonStress,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies stress test shutdown using power button",
		Contacts: []string{
			"intel.chrome.automation.team@intel.com",
			"pathan.jilani@intel.com",
		},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Convertible, hwdep.Clamshell, hwdep.Detachable)),
		Vars: []string{"servo",
			"power.mode", // Optional. Expecting "tablet". By default power.mode will be "clamshell".
		},
		Params: []testing.Param{
			{
				Name:      "quick",
				Val:       2,
				ExtraAttr: []string{"group:mainline", "informational", "group:intel-convertible"},
				Timeout:   5 * time.Minute,
			},
			{
				Name:      "bronze",
				Val:       10,
				ExtraAttr: []string{"group:intel-stress"},
				Timeout:   20 * time.Minute,
			}, {
				Name:    "silver",
				Val:     15,
				Timeout: 30 * time.Minute,
			}, {
				Name:    "gold",
				Val:     20,
				Timeout: 40 * time.Minute,
			},
		}})

}

// ShutdownPwrbuttonStress Perform shutdown using power button.
func ShutdownPwrbuttonStress(ctx context.Context, s *testing.State) {
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	dut := s.DUT()
	servoHostPort, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoHostPort, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	defer pxy.Close(ctxForCleanUp)
	iterations := s.Param().(int)

	tmc := &tabletmode.ConvertibleModeControl{}
	if err := tmc.InitControl(ctx, dut); err != nil {
		s.Fatal("Failed to init TabletModeControl: ", err)
	}
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Resetting tabletmode")
		if err := tmc.Reset(ctx); err != nil {
			s.Fatal("Failed to restore tabletmode to the original settings: ", err)
		}
	}(ctxForCleanUp)

	// Force DUT into tablet mode.
	testing.ContextLog(ctx, "Put DUT into tablet mode")
	if err := tmc.ForceTabletMode(ctx); err != nil {
		s.Fatal("Failed to set DUT into tablet mode: ", err)
	}

	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			if err := pwrOnDut(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to login to power on DUT: ", err)
			}
		}
	}(ctxForCleanUp)

	if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
		s.Fatal("Failed to login to chrome: ", err)
	}

	for i := 1; i <= iterations; i++ {
		s.Logf("Iteration: %d / %d", i, iterations)
		if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurLongPress); err != nil {
			s.Fatal("Failed to power long press: ", err)
		}

		sdCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		if err := dut.WaitUnreachable(sdCtx); err != nil {
			s.Fatal("Failed to shutdown DUT within 8 sec: error: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			pwrState, err := pxy.Servo().GetECSystemPowerState(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get power state G3 error")
			}
			if pwrState != "G3" {
				return errors.New("System is not in G3")
			}
			return nil
		}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
			s.Fatal("DUT failed to enter G3 state: ", err)
		}

		if err := pwrOnDut(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to login to power on DUT: ", err)
		}

		if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
			s.Fatal("Failed to login to chrome: ", err)
		}

		valid, err := powercontrol.IsPrevSleepStateAvailable(ctx, dut)

		if err != nil {
			s.Fatal("Failed to determine if prev_sleep_state is available: ", err)
		}

		if valid {
			if err := powercontrol.ValidatePrevSleepState(ctx, dut, 5); err != nil {
				s.Fatal("Previous Sleep state is not 5: ", err)
			}
		}
	}
}

// pwrOnDut power on DUT after pwr button long press.
func pwrOnDut(ctx context.Context, pxy *servo.Proxy, d *dut.DUT) error {
	if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
		return errors.Wrap(err, "failed to power normal press")
	}
	wtCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err := d.WaitConnect(wtCtx); err != nil {
		if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			return errors.Wrap(err, "failed to power normal press")
		}
		if err := d.WaitConnect(wtCtx); err != nil {
			return errors.Wrap(err, "failed to wait connect DUT")
		}
	}
	return nil
}
