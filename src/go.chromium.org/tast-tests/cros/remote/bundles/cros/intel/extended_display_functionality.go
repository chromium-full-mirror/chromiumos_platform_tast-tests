// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type extendedDisplayFunctionTestParams struct {
	powerMode              string
	ecStateToCheck         string
	expectedPrevSleepState int
	tabletmode             bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtendedDisplayFunctionality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies type-C extended display functionality before and after performing cold boot and warm boot",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome", "reboot"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.InternalDisplay()),
		Attr:         []string{"group:intel-hdmi-type-c"},
		Timeout:      5 * time.Minute,
		Vars:         []string{"servo"},
		Params: []testing.Param{{
			Name: "typec_hdmi_shutdown",
			Val: extendedDisplayFunctionTestParams{
				powerMode:              "shutdown_command",
				ecStateToCheck:         "S5",
				expectedPrevSleepState: 5,
				tabletmode:             false,
			},
		}, {
			Name: "typec_hdmi_reboot_tablet",
			Val: extendedDisplayFunctionTestParams{
				powerMode:              "reboot_command",
				expectedPrevSleepState: 0,
				tabletmode:             true,
			},
		}, {
			Name: "typec_hdmi_reboot_clamshell",
			Val: extendedDisplayFunctionTestParams{
				powerMode:              "reboot_command",
				expectedPrevSleepState: 0,
				tabletmode:             false,
			},
		}, {
			Name: "typec_hdmi_powerbtn_tablet",
			Val: extendedDisplayFunctionTestParams{
				powerMode:              "powerbtn_shutdown",
				ecStateToCheck:         "S5",
				expectedPrevSleepState: 5,
				tabletmode:             true,
			},
		}, {
			Name: "typec_hdmi_powerbtn_clamshell",
			Val: extendedDisplayFunctionTestParams{
				powerMode:              "powerbtn_shutdown",
				ecStateToCheck:         "S5",
				expectedPrevSleepState: 5,
				tabletmode:             false,
			},
		}},
	})
}

func ExtendedDisplayFunctionality(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	servoSpec, _ := s.Var("servo")
	dut := s.DUT()
	testOpt := s.Param().(extendedDisplayFunctionTestParams)

	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Get the initial tablet_mode_angle settings to restore at the end of test.
	re := regexp.MustCompile(`tablet_mode_angle=(\d+) hys=(\d+)`)
	tabletOut, err := dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle").Output()
	if err != nil {
		s.Fatal("Failed to retrieve tablet_mode_angle settings: ", err)
	}
	m := re.FindSubmatch(tabletOut)
	if len(m) != 3 {
		s.Fatalf("Failed to get initial tablet_mode_angle settings: got submatches %+v", m)
	}
	initLidAngle := m[1]
	initHys := m[2]

	if testOpt.tabletmode {
		// Set tabletModeAngle to 0 to force the DUT into tablet mode.
		testing.ContextLog(ctx, "Put DUT into tablet mode")
		if err := dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", "0", "0").Run(); err != nil {
			s.Fatal("Failed to set DUT into tablet mode: ", err)
		}
	}

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power on DUT at cleanup: ", err)
			}
		}
		if err := dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", string(initLidAngle), string(initHys)).Run(); err != nil {
			s.Fatal("Failed to restore tablet_mode_angle to the original settings: ", err)
		}
	}(cleanupCtx)

	// Perform a Chrome login.
	s.Log("Login to Chrome")
	if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
		s.Fatal("Failed to login to chrome: ", err)
	}

	// Verifying external display detection before shutdown/reboot.
	numberOfDisplay := 1
	spec := usbutils.DisplaySpec{
		NumberOfDisplays: &numberOfDisplay,
		DisplayType:      usbutils.TypeCHDMI,
	}
	if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, spec); err != nil {
		s.Fatal("Failed to detect external HDMI display: ", err)
	}

	if testOpt.powerMode == "shutdown_command" {
		testing.ContextLog(ctx, "Executing shutdown command")
		if err := powercontrol.ShutdownAndWaitForPowerState(ctx, pxy, dut, testOpt.ecStateToCheck); err != nil {
			s.Fatal("Failed to shutdown DUT and check EC state: ", err)
		}
	}

	if testOpt.powerMode == "reboot_command" {
		testing.ContextLog(ctx, "Rebooting the DUT")
		if err := performReboot(ctx, dut); err != nil {
			s.Fatal("Failed to perform DUT reboot: ", err)
		}
	}

	if testOpt.powerMode == "typec_hdmi_powerbtn" {
		testing.ContextLog(ctx, "Shutdown DUT with power button long press via servo")
		if err := shutdownWithPowerButtonViaServo(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to shutdown DUT with power long press via servo: ", err)
		}
		if err := verifyECPowerState(ctx, pxy, testOpt.ecStateToCheck); err != nil {
			s.Fatal("Failed to check EC power state: ", err)
		}
	}

	if testOpt.ecStateToCheck != "" {
		// Performing power normal press to power ON DUT.
		if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to power on DUT: ", err)
		}
	}

	// Verifying external display detection after shutdown/reboot.
	if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, spec); err != nil {
		s.Fatalf("Failed detecting external display after %q: %v", testOpt.powerMode, err)
	}

	// Performing prev_sleep_state check.
	valid, err := powercontrol.IsPrevSleepStateAvailable(ctx, dut)

	if err != nil {
		s.Fatal("Failed to determine if prev_sleep_state is available: ", err)
	}

	if valid {
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, testOpt.expectedPrevSleepState); err != nil {
			s.Fatal("Failed to validate previous sleep state: ", err)
		}
	}
}

// shutdownWithPowerButtonViaServo performs shutdown DUT with power button long press via servo.
func shutdownWithPowerButtonViaServo(ctx context.Context, pxy *servo.Proxy, dut *dut.DUT) error {
	if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurLongPress); err != nil {
		return errors.Wrap(err, "failed to power long press")
	}
	sdCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		return errors.Wrap(err, "failed to wait DUT to be unreachable")
	}
	return nil
}

// verifyECPowerState verifies EC powerState of DUT.
func verifyECPowerState(ctx context.Context, pxy *servo.Proxy, powerState string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		got, err := pxy.Servo().GetECSystemPowerState(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get EC power state")
		}
		if want := powerState; got != want {
			return errors.Errorf("unexpected DUT EC power state = got %q, want %q", got, want)
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second})
}

// performReboot performs DUT reboot.
func performReboot(ctx context.Context, dut *dut.DUT) error {
	if err := dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT")
	}
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := dut.WaitConnect(waitCtx); err != nil {
		return errors.Wrap(err, "failed to wait connect to DUT")
	}
	return nil
}
