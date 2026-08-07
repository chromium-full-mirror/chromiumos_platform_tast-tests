// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shimlessrma contains integration tests for Shimless RMA SWA.
package shimlessrma

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	servo "go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/shimlessrma/rmaweb"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/shimlessrma/servoutil"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type param struct {
	wp          rmaweb.WriteProtectDisableOption
	enroll      bool
	destination rmaweb.DestinationOption
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DisableHWWP,
		Desc: "Can complete Shimless RMA successfully. Disable HWWP with Battery Disconnection",
		Contacts: []string{
			"chromeos-shimless-eng@google.com",
			"chenghan@google.com",
		},
		BugComponent: "b:1002147",
		Attr:         []string{"group:shimless_rma"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		SoftwareDeps: []string{"chrome", "gsc", "reboot", "tpm_clear_allowed"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.GSCUART(), hwdep.Battery(), hwdep.MinStorage(16)),
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.shimlessrma.AppService",
			"tast.cros.graphics.ScreenshotService",
		},
		Fixture: fixture.DevModeGBB,
		Timeout: 150 * time.Minute,
		Params: []testing.Param{{
			ExtraAttr: []string{"shimless_rma_normal"},
			Name:      "unenroll_sameuser_manual",
			Val: param{
				wp:          rmaweb.Manual,
				enroll:      false,
				destination: rmaweb.SameUser,
			},
		}, {
			ExtraAttr: []string{"shimless_rma_nodelocked"},
			Name:      "unenroll_sameuser_rsu",
			Val: param{
				wp:          rmaweb.Rsu,
				enroll:      false,
				destination: rmaweb.SameUser,
			},
		}, {
			ExtraAttr: []string{"shimless_rma_nodelocked"},
			Name:      "unenroll_diffuser_rsu",
			Val: param{
				wp:          rmaweb.Rsu,
				enroll:      false,
				destination: rmaweb.DifferentUser,
			},
		}, {
			ExtraAttr: []string{"shimless_rma_nodelocked"},
			Name:      "enroll_diffuser_rsu",
			Val: param{
				wp:          rmaweb.Rsu,
				enroll:      true,
				destination: rmaweb.DifferentUser,
			},
		}, {
			ExtraAttr: []string{"shimless_rma_pretest"},
			Name:      "unenroll_sameuser_manual_pretest",
			Val: param{
				wp:          rmaweb.Manual,
				enroll:      false,
				destination: rmaweb.SameUser,
			},
		}},
	})
}

func DisableHWWP(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	firmwareHelper := s.FixtValue().(*fixture.Value).Helper
	dut := firmwareHelper.DUT
	p := s.Param().(param)
	wpOption := p.wp
	enroll := p.enroll
	destination := p.destination
	bypassRacc := !isRaccEnabled(s)

	// TODO(b/349959175): Test firmware update from rootfs.
	firmwareUpdateOption := rmaweb.FirmwareUpdateOptionSkip

	defer rmaweb.CleanupShimlessFiles(cleanupCtx, dut)

	if err := firmwareHelper.RequireServo(ctx); err != nil {
		s.Fatal("Fail to init servo: ", err)
	}

	uiHelper, err := rmaweb.NewUIHelper(ctx, s, &rmaweb.UIHelperOptions{
		KeepState:  true,
		BypassRacc: bypassRacc,
	})
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}
	// Restart will dispose resources, so don't dispose resources explicitly.
	errorHandler := rmaweb.CreateErrorHandler(ctx, &uiHelper, s.TestName())
	s.AttachErrorHandlers(errorHandler, errorHandler)

	if err := uiHelper.SetupInitStatus(ctx, enroll); err != nil {
		s.Fatal("Fail to setup init status: ", err)
	}

	// Leaving factory mode will reset capabilities so we have to explicitly allow unverified RO to prevent brick.
	if err := setAllowUnverifiedRoToAlways(ctx, firmwareHelper, true); err != nil {
		s.Fatal("Fail to reset AllowUnverifiedRo to Always: ", err)
	}

	uiHelper, err = rmaweb.NewUIHelper(ctx, s, &rmaweb.UIHelperOptions{
		KeepState:  false,
		BypassRacc: bypassRacc,
	})
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}
	// Restart will dispose resources, so don't dispose resources explicitly.

	if actions := generateActionCombinedToDisableWP(wpOption, enroll, destination, uiHelper); actions == nil {
		// We don't support this test case yet.
		s.Fatalf("The test case is not support yet. Enroll: %t, WP: %s, destination: %s ", enroll, wpOption, destination)
	} else {
		if err := actions(ctx); err != nil {
			s.Fatal("Fail to navigate to Disable Write Protect page and turn off write protect: ", err)
		}
	}

	// GoBigSleepLint: Wait for reboot start.
	// TODO(chenghan): Replace testing.Sleep with testing.Poll.
	if err := testing.Sleep(ctx, rmaweb.WaitForRebootStart); err != nil {
		s.Error("Fail to sleep: ", err)
	}

	uiHelper, err = rmaweb.NewUIHelper(ctx, s, &rmaweb.UIHelperOptions{
		KeepState:  true,
		BypassRacc: bypassRacc,
	})
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}
	// Restart will dispose resources, so don't dispose resources explicitly.

	if firmwareUpdateOption == rmaweb.FirmwareUpdateOptionSkip {
		if err := uiHelper.BypassFirmwareInstallation(ctx); err != nil {
			s.Fatal("Fail to update state file to bypass firmware install: ", err)
		}
	}

	uiHelper, err = rmaweb.NewUIHelper(ctx, s, &rmaweb.UIHelperOptions{
		KeepState:  true,
		BypassRacc: bypassRacc,
	})
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}
	// Restart will dispose resources, so don't dispose resources explicitly.

	if err := uiHelper.WriteProtectDisabledPageOperation(ctx); err != nil {
		s.Fatal("Fail to navigate to WP disable Complete page: ", err)
	}

	// GoBigSleepLint: Wait for reboot start.
	if err := testing.Sleep(ctx, rmaweb.WaitForRebootStart); err != nil {
		s.Error("Fail to sleep: ", err)
	}

	uiHelper, err = rmaweb.NewUIHelper(ctx, s, &rmaweb.UIHelperOptions{
		KeepState:  true,
		BypassRacc: bypassRacc,
	})
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}

	if err := uiHelper.DeviceInformationPageOperation(ctx); err != nil {
		s.Fatal("Fail to complete Device Information page operations: ", err)
	}

	// Always bypass calibration in this test. We have another test just for calibration.
	if err := uiHelper.BypassCalibration(ctx); err != nil {
		s.Fatal("Fail to bypass calibration after firmware installation: ", err)
	}

	if firmwareUpdateOption != rmaweb.FirmwareUpdateOptionSkip {
		if err := uiHelper.WaitForFirmwareInstallation(ctx); err != nil {
			s.Fatal("Fail to navigate to firmware installation page and install firmware: ", err)
		}
	}

	// GoBigSleepLint: Wait for reboot start.
	if err := testing.Sleep(ctx, rmaweb.WaitForRebootStart); err != nil {
		s.Error("Fail to sleep: ", err)
	}

	// Given the flakiness of USB on the lab devices, the USB may malfunction at the first place.
	// We should let the test continue if the USB is already unseen to the DUT.
	if err := firmwareHelper.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		s.Log("Fail to set USB Mux state: ", err)
	}

	if err := setAllowUnverifiedRoToAlways(ctx, firmwareHelper, false); err != nil {
		s.Fatal("Fail to reset AllowUnverifiedRo to Always: ", err)
	}

	if err := rmaweb.PollStateField(ctx, s, rmaweb.RmadStateFieldFinalizeRebooted, true, rmaweb.StateFieldPollingTimeout); err != nil {
		s.Fatal("Fail to wait for finalize reboot: ", err)
	}

	uiHelper, err = rmaweb.NewUIHelper(ctx, s, &rmaweb.UIHelperOptions{
		KeepState:  true,
		BypassRacc: bypassRacc,
	})
	if err != nil {
		s.Fatal("Fail to initialize RMA Helper: ", err)
	}

	defer uiHelper.DisposeResource(cleanupCtx)

	if err := uiHelper.RepairCompletedPageOperation(ctx); err != nil {
		s.Fatal("Fail to navigate to Repair Complete page: ", err)
	}
}

func isRaccEnabled(s *testing.State) bool {
	features := s.Features("").Hardware
	cond := hwdep.RuntimeProbeConfig()
	satisfied, reason, err := cond.Satisfied(features)
	if err != nil {
		s.Log("isRaccEnabled: failed to check RuntimeProbeConfig condition: ", err)
		return false
	}

	if !satisfied {
		s.Log("RuntimeProbeConfig not present: ", reason)
		return false
	}

	s.Log("RuntimeProbeConfig is present")
	return true
}

func generateActionCombinedToDisableWP(option rmaweb.WriteProtectDisableOption, enroll bool, destination rmaweb.DestinationOption, uiHelper *rmaweb.UIHelper) action.Action {

	if enroll && destination == rmaweb.DifferentUser && option == rmaweb.Rsu {
		return action.Combine("navigate to RSU page and turn off write protect",
			uiHelper.WelcomePageOperation,
			uiHelper.ComponentsPageOperation,
			uiHelper.OwnerPageOperation(destination),
			uiHelper.RSUPageOperation,
		)
	} else if !enroll && destination == rmaweb.DifferentUser && option == rmaweb.Rsu {
		return action.Combine("navigate to RSU page, choose different user and turn off write protect",
			uiHelper.WelcomePageOperation,
			uiHelper.ComponentsPageOperation,
			uiHelper.OwnerPageOperation(destination),
			uiHelper.WriteProtectPageChooseRSU,
			uiHelper.RSUPageOperation,
		)
	} else if !enroll && destination == rmaweb.SameUser && option == rmaweb.Rsu {
		return action.Combine("navigate to RSU page , choose same user and turn off write protect",
			uiHelper.WelcomePageOperation,
			uiHelper.ComponentsPageOperation,
			uiHelper.OwnerPageOperation(destination),
			uiHelper.WriteProtectPageChooseRSU,
			uiHelper.RSUPageOperation,
		)
	} else if !enroll && destination == rmaweb.SameUser && option == rmaweb.Manual {
		return action.Combine("navigate to Manual Disable Write Protect page, choose same user and turn off write protect",
			uiHelper.WelcomePageOperation,
			uiHelper.ComponentsPageOperation,
			uiHelper.OwnerPageOperation(destination),
			uiHelper.WriteProtectPageChooseManual,
		)
	}

	return nil
}

func setAllowUnverifiedRoToAlways(ctx context.Context, firmwareHelper *firmware.Helper, forceReboot bool) error {
	// We set AllowUnverifiedRo to Always because lab devices are installed with dev-signed firmware,
	// which cannot pass APROV, and will be held in reset by GSC.
	if isTi50, err := servoutil.IsTi50(ctx, firmwareHelper); err != nil || !isTi50 {
		// We still try to complete the rest of test because setting capabilities is not what we want to
		// verify with this test.
		testing.ContextLog(ctx, "Fail to determine if the device is a Ti50 device or it is not")
		if !forceReboot {
			return nil
		}
		return firmwareHelper.DUT.Reboot(ctx)
	}

	// Wait rmad for leaving factory mode.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, accessible, err := firmwareHelper.Servo.GetCCDCapability(ctx, servo.AllowUnverifiedRo); err != nil {
			return errors.Wrap(err, "failed to get AllowUnverifiedRo state")
		} else if accessible == "Y" {
			return errors.New("AllowUnverifiedRo is still Always")
		}
		return nil
	}, &testing.PollOptions{Timeout: 1 * time.Minute, Interval: 10 * time.Second}); err != nil {
		// Informational log. We will still try to reset AllowUnverifiedRo.
		testing.ContextLog(ctx, "Failed to wait for AllowUnverifiedRo being set to Default")
	}

	// Open CCD again because it is locked by leaving factory mode.
	if err := firmwareHelper.OpenCCD(ctx /*ensureTestlab=*/, true /*resetCCD=*/, false); err != nil {
		return errors.Wrap(err, "failed to open CCD")
	}

	// Set AllowUnverifiedRo back to Always to avoid holding devices in reset.
	ccdSettings := map[servo.CCDCap]servo.CCDCapState{servo.AllowUnverifiedRo: servo.CapAlways}
	if err := firmwareHelper.Servo.SetCCDCapability(ctx, ccdSettings); err != nil {
		return errors.Wrap(err, "failed to reset AllowUnverifiedRo")
	}

	// Reboot GSC to make AllowUnverifiedRo take effect.
	if err := servoutil.RebootGSC(ctx, firmwareHelper); err != nil {
		return errors.Wrap(err, "failed to reboot GSC")
	}

	return nil
}
