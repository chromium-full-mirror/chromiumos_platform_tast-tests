// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type sdCardTestParams struct {
	powerMode       string
	cbmemSleepState int
	iterCount       int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SDCardFunctionality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies micro SD card functionality before and after performing, shutdown or reboot",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome", "reboot"},
		Vars:         []string{"servo", "intel.sdCardDetectionName"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Attr:         []string{"group:intel-usb-set1"},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name: "shutdown",
			Val: sdCardTestParams{
				powerMode:       "shutdown",
				cbmemSleepState: 5, // cbmemSleepState must be 5 for coldboot validation.
				iterCount:       1,
			},
		}, {

			Name: "shutdown_stress",
			Val: sdCardTestParams{
				powerMode:       "shutdown",
				cbmemSleepState: 5, // cbmemSleepState must be 5 for coldboot validation.
				iterCount:       10,
			},
		}, {

			Name: "reboot",
			Val: sdCardTestParams{
				powerMode:       "reboot",
				cbmemSleepState: 0, // cbmemSleepState must be 0 for warmboot validation.
				iterCount:       1,
			},
		}},
	})
}

// SDCardFunctionality checks SD card functionality while performing shutdown and reboot.
// Pre-requisite: SD card must be inserted into DUT SD card slot.
func SDCardFunctionality(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	sdCardName := s.RequiredVar("intel.sdCardDetectionName")

	dut := s.DUT()
	servoSpec, ok := s.Var("servo")
	if !ok {
		servoSpec = ""
	}

	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	testOpt := s.Param().(sdCardTestParams)

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to wake up DUT: ", err)
		}
	}(cleanupCtx)

	// Performing chrome intital login.
	if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
		s.Fatal("Failed to login to chrome: ", err)
	}

	if err := utils.PlugSDCardViaServo(ctx, pxy); err != nil {
		s.Fatal("Failed to plug SD card via servo: ", err)
	}
	defer utils.UnplugSDCardViaServo(cleanupCtx, pxy)

	for i := 1; i <= testOpt.iterCount; i++ {
		s.Logf("Iteration: %d/%d", i, testOpt.iterCount)
		if err := utils.WaitForSDCardDetection(ctx, dut, sdCardName); err != nil {
			s.Fatal("Failed to detect SD card: ", err)
		}

		if testOpt.powerMode == "shutdown" {
			powerState := "S5"
			if err := powercontrol.ShutdownAndWaitForPowerState(ctx, pxy, dut, powerState); err != nil {
				s.Fatalf("Failed to shutdown and wait for %q powerstate: %v", powerState, err)
			}

			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to wake up DUT: ", err)
			}
		}

		if testOpt.powerMode == "reboot" {
			if err := dut.Reboot(ctx); err != nil {
				s.Fatal("Failed to reboot DUT: ", err)
			}
			waitCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
			defer cancel()
			if err := dut.WaitConnect(waitCtx); err != nil {
				s.Fatal("Failed to wait connect DUT after reboot: ", err)
			}
		}

		// Performing chrome login after powering on DUT from coldboot/warmboot.
		if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
			s.Fatalf("Failed to login to chrome after %q: %v", testOpt.powerMode, err)
		}

		if err := powercontrol.ValidatePrevSleepState(ctx, dut, testOpt.cbmemSleepState); err != nil {
			s.Fatal("Failed to validate previous sleep state: ", err)
		}
	}
}
