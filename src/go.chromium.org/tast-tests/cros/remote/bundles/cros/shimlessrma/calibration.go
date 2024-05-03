// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shimlessrma contains integration tests for Shimless RMA SWA.
package shimlessrma

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/shimlessrma/rmaweb"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type sensor struct {
	name          string
	stateFilePath string
}

var sensorAccel = sensor{
	name:          "accel",
	stateFilePath: "state_files/lid_accel_calibration.json",
}

var sensorGyro = sensor{
	name:          "gyro",
	stateFilePath: "state_files/base_gyro_calibration.json",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Calibration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test accelerometer/gyro calibration in Shimless RMA",
		Contacts: []string{
			"chromeos-shimless-eng@google.com",
			"chenghan@google.com",
		},
		BugComponent: "b:1002147",
		Attr:         []string{"group:shimless_rma", "shimless_rma_calibration"},
		Data:         []string{sensorAccel.stateFilePath, sensorGyro.stateFilePath},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Convertible)),
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.shimlessrma.AppService",
			"tast.cros.graphics.ScreenshotService",
		},
		Fixture: fixture.DevModeGBB,
		Timeout: 10 * time.Minute,
		Params: []testing.Param{{
			Name: "accel",
			Val:  sensorAccel,
		}, {
			Name: "gyro",
			Val:  sensorGyro,
		}},
	})
}

func Calibration(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	firmwareHelper := s.FixtValue().(*fixture.Value).Helper
	dut := firmwareHelper.DUT
	key := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	defer rmaweb.CleanupShimlessFiles(cleanupCtx, dut)

	if err := firmwareHelper.RequireServo(ctx); err != nil {
		s.Fatal("Fail to init servo: ", err)
	}

	uiHelper, err := rmaweb.NewUIHelper(ctx, dut, firmwareHelper, s.RPCHint(), key, false)
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}
	errorHandler := rmaweb.CreateErrorHandler(ctx, &uiHelper, s.TestName())
	s.AttachErrorHandlers(errorHandler, errorHandler)

	if err := uiHelper.SetupInitStatus(ctx, false); err != nil {
		s.Fatal("Fail to setup init status: ", err)
	}

	uiHelper, err = rmaweb.NewUIHelper(ctx, dut, firmwareHelper, s.RPCHint(), key, true)
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}

	if err := generateActionCombinedToDisableWPManual(uiHelper)(ctx); err != nil {
		s.Fatal("Fail to navigate to Disable Write Protect page and turn off write protect: ", err)
	}

	// GoBigSleepLint: Wait for reboot to enable the factory mode, so that we don't need to enable
	// HWWP ourselves later. The device should shutdown in |WaitForRebootStart| seconds.
	if err := testing.Sleep(ctx, rmaweb.WaitForRebootStart); err != nil {
		s.Fatal("Fail to sleep to wait for reboot to enter factory mode: ", err)
	}

	uiHelper, err = rmaweb.NewUIHelper(ctx, dut, firmwareHelper, s.RPCHint(), key, true)
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}

	component := s.Param().(sensor)
	statePath := s.DataPath(component.stateFilePath)

	s.Logf("The path of state file is: %s", statePath)

	b, err := os.ReadFile(statePath)
	if err != nil {
		s.Fatal("Fail to read state file on host: ", err)
	}
	stateFileContent := string(b)

	if err := generateActionCombinedToGetToCalibration(uiHelper, stateFileContent)(ctx); err != nil {
		s.Fatal("Fail to override state file: ", err)
	}

	// GoBigSleepLint: Wait for reboot start.
	if err := testing.Sleep(ctx, rmaweb.WaitForRebootStart); err != nil {
		s.Fatal("Fail to sleep to wait for reboot start: ", err)
	}

	uiHelper, err = rmaweb.NewUIHelper(ctx, dut, firmwareHelper, s.RPCHint(), key, true)
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}
	defer uiHelper.DisposeResource(cleanupCtx)

	if component == sensorAccel {
		if err := uiHelper.CalibrateLidAccelerometerPageOperation(ctx); err != nil {
			s.Fatal("Fail to calibrate lid accelerometer: ", err)
		}
	} else {
		if err := uiHelper.CalibrateBaseGyroPageOperation(ctx); err != nil {
			s.Fatal("Fail to calibrate base gyro: ", err)
		}
	}

	if err := uiHelper.RepairCompletedPageOperation(ctx, rmaweb.NotStoreLog); err != nil {
		s.Fatal("Fail to navigate to Repair Complete page: ", err)
	}
}

func generateActionCombinedToDisableWPManual(uiHelper *rmaweb.UIHelper) action.Action {
	return action.Combine("navigate to Manual Disable Write Protect page, choose same user and turn off write protect",
		uiHelper.WelcomePageOperation,
		uiHelper.ComponentsPageOperation,
		uiHelper.OwnerPageOperation(rmaweb.SameUser),
		uiHelper.WriteProtectPageChooseManual,
	)
}

func generateActionCombinedToGetToCalibration(uiHelper *rmaweb.UIHelper, content string) action.Action {
	return action.Combine("wait for HWWP disabled and get to calibration page",
		uiHelper.WaitForHWWPDisableCompletePage,
		func(ctx context.Context) error {
			return uiHelper.OverrideStateFile(ctx, content)
		},
	)
}
