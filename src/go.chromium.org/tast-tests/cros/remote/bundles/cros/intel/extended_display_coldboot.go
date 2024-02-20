// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type extendedDisplayTestParams struct {
	ecStateToCheck string
	displayType    string
	iterationCount int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtendedDisplayColdboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies extended display functionality before and after performing cold boot",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		SoftwareDeps: []string{"chrome", "reboot"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.InternalDisplay()),
		Vars:         []string{"servo"},
		Params: []testing.Param{{
			Name: "typec_dp_g3",
			Val: extendedDisplayTestParams{
				ecStateToCheck: "G3",
				displayType:    usbutils.TypeCDP,
				iterationCount: 1,
			},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-dp-type-c"},
		}, {
			Name: "typec_dp_s5",
			Val: extendedDisplayTestParams{
				ecStateToCheck: "S5",
				displayType:    usbutils.TypeCDP,
				iterationCount: 10,
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"group:intel-dp-type-c"},
		}, {
			Name: "native_dp_s5",
			Val: extendedDisplayTestParams{
				ecStateToCheck: "S5",
				displayType:    usbutils.NativeDP,
				iterationCount: 10,
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"group:intel-dp"},
		}, {
			Name: "native_dp_g3",
			Val: extendedDisplayTestParams{
				ecStateToCheck: "G3",
				displayType:    usbutils.NativeDP,
				iterationCount: 10,
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"group:intel-dp"},
		}, {
			Name: "typec_hdmi_g3",
			Val: extendedDisplayTestParams{
				ecStateToCheck: "G3",
				displayType:    usbutils.TypeCHDMI,
				iterationCount: 10,
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"group:intel-hdmi-type-c"},
		}, {
			Name: "typec_hdmi_s5",
			Val: extendedDisplayTestParams{
				ecStateToCheck: "S5",
				displayType:    usbutils.TypeCHDMI,
				iterationCount: 1,
			},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-hdmi-type-c"},
		}, {
			Name: "native_hdmi_g3",
			Val: extendedDisplayTestParams{
				ecStateToCheck: "G3",
				displayType:    usbutils.NativeHDMI,
				iterationCount: 10,
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"group:intel-hdmi"},
		}, {
			Name: "native_hdmi_s5",
			Val: extendedDisplayTestParams{
				ecStateToCheck: "S5",
				displayType:    usbutils.NativeHDMI,
				iterationCount: 10,
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"group:intel-hdmi"},
		}},
	})
}

func ExtendedDisplayColdboot(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	servoSpec, _ := s.Var("servo")
	dut := s.DUT()
	testOpt := s.Param().(extendedDisplayTestParams)

	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power on DUT at cleanup: ", err)
			}
		}
	}(cleanupCtx)

	// Login to chrome and check for connected external display
	loginChrome := func(ctx context.Context) {
		// Perform a Chrome login.
		if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
			s.Fatal("Failed to login to Chrome: ", err)
		}
		numberOfDisplay := 1
		spec := usbutils.DisplaySpec{
			NumberOfDisplays: &numberOfDisplay,
			DisplayType:      testOpt.displayType,
		}
		if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, spec); err != nil {
			s.Fatalf("Failed to detect external %s display: %v", testOpt.displayType, err)
		}
	}

	loginChrome(ctx)

	for i := 1; i <= testOpt.iterationCount; i++ {
		s.Logf("Iteration: %d/%d ", i, testOpt.iterationCount)
		if err := powercontrol.ShutdownAndWaitForPowerState(ctx, pxy, dut, testOpt.ecStateToCheck); err != nil {
			s.Fatalf("Failed to shutdown and wait for %q powerstate: %v", testOpt.ecStateToCheck, err)
		}

		if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to power on DUT: ", err)
		}

		// Login chrome after waking from coldboot.
		loginChrome(ctx)

		valid, err := powercontrol.IsPrevSleepStateAvailable(ctx, dut)

		if err != nil {
			s.Fatal("Failed to determine if prev_sleep_state is available: ", err)
		}

		if valid {
			// Performing prev_sleep_state check.
			const expectedPrevSleepState = 5
			if err := powercontrol.ValidatePrevSleepState(ctx, dut, expectedPrevSleepState); err != nil {
				s.Fatal("Failed to validate previous sleep state: ", err)
			}
		}
	}
}
