// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package rmaweb contains web-related common functions used in the Shimless RMA app.
package rmaweb

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/shimlessrma/servoutil"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pb "go.chromium.org/tast-tests/cros/services/cros/shimlessrma"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// DestinationOption indicates destination.
type DestinationOption string

// WriteProtectDisableOption indicates write protection disabling approach.
type WriteProtectDisableOption string

// FirmwareUpdateOption indicates the option to flash AP RO firmware.
type FirmwareUpdateOption string

// StoreLogFlag indicates whether to store log to USB during test.
type StoreLogFlag bool

// RmadStateField indicates the keys in rmad state JSON.
type RmadStateField string

const (
	// SameUser indicates devices goes to same user.
	SameUser DestinationOption = "SAME_USER"

	// DifferentUser indicates devices goes to different user.
	DifferentUser DestinationOption = "DIFFERENT_USER"

	// Manual indicates using battery disconnection to disable write protect.
	Manual WriteProtectDisableOption = "MANUAL"

	// Rsu indicates using rsu to disable write protect.
	Rsu WriteProtectDisableOption = "RSU"

	// FirmwareUpdateOptionRootfs indicates flashing AP RO firmware from RootFS.
	FirmwareUpdateOptionRootfs FirmwareUpdateOption = "FIRMWARE_UPDATE_SOURCE_ROOTFS"

	// FirmwareUpdateOptionUsb indicates flashing AP RO firmware from USB drive.
	FirmwareUpdateOptionUsb FirmwareUpdateOption = "FIRMWARE_UPDATE_SOURCE_USB"

	// FirmwareUpdateOptionSkip indicates skipping flashing AP RO firmwarm.
	FirmwareUpdateOptionSkip FirmwareUpdateOption = "FIRMWARE_UPDATE_SOURCE_SKIP"

	// RmadStateFieldFinalizeRebooted indicates whether the device has rebooted during
	// the finalizing state.
	RmadStateFieldFinalizeRebooted RmadStateField = "finalize_rebooted"

	// WaitForRebootStart indicates the time to wait before reboot starting.
	WaitForRebootStart = 10 * time.Second

	// WaitForProvisionAndFinalize indicates the time to wait for Shimless RMA finishing
	// provisioning and finalizing.
	WaitForProvisionAndFinalize = 30 * time.Second

	// StateFieldPollingTimeout indicates the time to wait for a field value being changed
	// to an expected value.
	StateFieldPollingTimeout = 90 * time.Second

	// StoreLog indicates that log will be stored into usb during test.
	StoreLog StoreLogFlag = true

	// NotStoreLog indicates that log will not be stored into usb during test.
	NotStoreLog StoreLogFlag = false

	timeInSecondToLoadPage         = 30
	timeInSecondToEnableButton     = 5
	longTimeInSecondToEnableButton = 60
	firmwareInstallationTime       = 240 * time.Second
	stateFileUpdateTime            = 3 * time.Second
	usbTempMountDir                = "/media/usb-drive"
	stateFile                      = "/mnt/stateful_partition/unencrypted/rma-data/state"
)

// UIHelper holds the resources required to communicate with Shimless RMA App.
type UIHelper struct {
	// Client contains Shimless RMA App client.
	Client         pb.AppServiceClient
	Dut            *dut.DUT
	FirmwareHelper *firmware.Helper
	RPCClient      *rpc.Client
}

// UIHelperOptions contains options to create a UIhelper.
type UIHelperOptions struct {
	KeepState  bool
	BypassRacc bool
}

// NewUIHelper creates UIHelper.
func NewUIHelper(ctx context.Context, s *testing.State, opts *UIHelperOptions) (*UIHelper, error) {
	firmwareHelper := s.FixtValue().(*fixture.Value).Helper
	dut := firmwareHelper.DUT
	key := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	rpcHint := s.RPCHint()

	if err := firmwareHelper.WaitConnect(ctx); err != nil {
		return nil, err
	}

	// Setup rpc.
	rpcClient, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return nil, err
	}

	request := &pb.NewShimlessRMARequest{
		ManifestKey: key,
		KeepState:   opts.KeepState,
		BypassRacc:  opts.BypassRacc,
	}
	client := pb.NewAppServiceClient(rpcClient.Conn)
	if _, err := client.NewShimlessRMA(ctx, request, grpc.WaitForReady(true)); err != nil {
		return nil, err
	}

	uiHelper := &UIHelper{client, dut, firmwareHelper, rpcClient}

	return uiHelper, nil
}

// DisposeResource will close the resources which are required in UIHelper.
func (uiHelper *UIHelper) DisposeResource(cleanupCtx context.Context) {
	if _, err := uiHelper.Client.CloseShimlessRMA(cleanupCtx, &empty.Empty{}); err != nil {
		testing.ContextLog(cleanupCtx, "Fail to close Shimless RMA client: ", err)
	}
	if err := uiHelper.RPCClient.Close(cleanupCtx); err != nil {
		testing.ContextLog(cleanupCtx, "Fail to close RPC client: ", err)
	}
	if err := uiHelper.Dut.Reboot(cleanupCtx); err != nil {
		testing.ContextLog(cleanupCtx, "Failed to reboot DUT: ")
	}
}

// WelcomePageOperation handles all operations on Welcome Page.
func (uiHelper *UIHelper) WelcomePageOperation(ctx context.Context) error {
	return action.Combine("welcome page operation",
		uiHelper.waitForPageToLoad("Chromebook repair", timeInSecondToLoadPage),
		uiHelper.waitAndClickButton("Get started", longTimeInSecondToEnableButton),
	)(ctx)
}

// PrepareOfflineTest prepares DUT for offline mode.
func (uiHelper *UIHelper) PrepareOfflineTest(ctx context.Context) error {
	_, err := uiHelper.Client.PrepareOfflineTest(ctx, &empty.Empty{})
	return err
}

// WelcomeAndNetworkPageOperationOffline handles all operations on Welcome Page and Network Connection Page in offline mode.
func (uiHelper *UIHelper) WelcomeAndNetworkPageOperationOffline(ctx context.Context, wifiName string) error {
	_, err := uiHelper.Client.TestWelcomeAndNetworkConnection(ctx, &pb.TestWelcomeAndNetworkConnectionRequest{
		WifiName: wifiName,
	})
	return err
}

// VerifyWifiIsForgotten verify that wifi is forgotten.
func (uiHelper *UIHelper) VerifyWifiIsForgotten(ctx context.Context) error {
	_, err := uiHelper.Client.VerifyNoWifiConnected(ctx, &empty.Empty{})
	return err
}

// VerifyOfflineOperationSuccess verify that offline operation is successful.
func (uiHelper *UIHelper) VerifyOfflineOperationSuccess(ctx context.Context) error {
	_, err := uiHelper.Client.VerifyTestWelcomeAndNetworkConnectionSuccess(ctx, &empty.Empty{})
	return err
}

// ComponentsPageOperation handles all operations on Components Selection Page.
func (uiHelper *UIHelper) ComponentsPageOperation(ctx context.Context) error {
	return action.Combine("components page operation",
		uiHelper.waitForPageToLoad("Select which components were replaced", timeInSecondToLoadPage),
		uiHelper.clickToggleButton("Base Accelerometer"),
		uiHelper.clickButton("Next"),
	)(ctx)
}

// OwnerPageOperation handles all operations on Owner Selection Page.
func (uiHelper *UIHelper) OwnerPageOperation(destination DestinationOption) action.Action {
	return func(ctx context.Context) error {
		var buttonLabel string
		if destination == SameUser {
			buttonLabel = "Device will go to the same user"
		} else if destination == DifferentUser {
			buttonLabel = "Device will go to a different user or organization"
		} else {
			return errors.Errorf("%s is invalid destination", destination)
		}

		return action.Combine("owner page operation",
			uiHelper.waitForPageToLoad("After repair, who will be using the device?", timeInSecondToLoadPage),
			uiHelper.clickRadioButton(buttonLabel),
			uiHelper.waitAndClickButton("Next", timeInSecondToEnableButton),
		)(ctx)
	}
}

// WriteProtectPageChooseRSU handles all operations on WP Page and select RSU.
func (uiHelper *UIHelper) WriteProtectPageChooseRSU(ctx context.Context) error {
	return uiHelper.writeProtectPageOperation("Perform RMA Server Unlock (RSU)")(ctx)
}

// WriteProtectPageChooseManual handles all operations on WP Page and select manual option.
func (uiHelper *UIHelper) WriteProtectPageChooseManual(ctx context.Context) error {
	return action.Combine("write Protect page operation and choose manual",
		uiHelper.writeProtectPageOperation("Manually turn off"),
		uiHelper.disconnectBatteryByCr50(),
	)(ctx)

}

// WriteProtectDisabledPageOperation handles all operations on Write Protect Disabled Page.
func (uiHelper *UIHelper) WriteProtectDisabledPageOperation(ctx context.Context) error {
	return action.Combine("write Protect Disabled page operation",
		uiHelper.WaitForHWWPDisableCompletePage,
		uiHelper.clickButton("Next"),
	)(ctx)
}

// WriteProtectEnabledPageOperation handles all operations on Write Protect Enable Page.
func (uiHelper *UIHelper) WriteProtectEnabledPageOperation(ctx context.Context) error {
	return action.Combine("write Protect Enabled page operation",
		uiHelper.waitForPageToLoad("Manually enable write-protect", timeInSecondToLoadPage),
		uiHelper.clickButton("Next"),
	)(ctx)
}

// DeviceInformationPageOperation handles all operations on device information Page.
func (uiHelper *UIHelper) DeviceInformationPageOperation(ctx context.Context) error {
	return action.Combine("device Information page operation",
		uiHelper.waitForPageToLoad("Please confirm device information", timeInSecondToLoadPage),
		uiHelper.clickButton("Next"),
	)(ctx)
}

// CalibrateLidAccelerometerPageOperation handles all operations on calibrate lid accelerometer Page.
func (uiHelper *UIHelper) CalibrateLidAccelerometerPageOperation(ctx context.Context) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// The first attempt will fail since we don't fake data yet.
	if err := action.Combine("calibrate lid accelerometer operation",
		uiHelper.waitForPageToLoad("Calibrate components", timeInSecondToLoadPage),
		uiHelper.waitAndClickButton("Next", longTimeInSecondToEnableButton),
		uiHelper.waitForPageToLoad("Calibrating components…", timeInSecondToLoadPage),
	)(ctx); err != nil {
		return err
	}

	output, _ := uiHelper.Dut.Conn().CommandContext(ctx, "ectool", "motionsense", "lid_angle").Output()
	testing.ContextLogf(ctx, "Lid angle using real data is %q", string(output))

	// fake data.
	if err := uiHelper.flattenDutWithFakeSensorData(ctx); err != nil {
		return errors.Wrap(err, "fail to fake sensor data")
	}
	// Reset to use real sensor data.
	defer uiHelper.resetToUseRealSensorData(cleanupCtx)

	output, _ = uiHelper.Dut.Conn().CommandContext(ctx, "ectool", "motionsense", "lid_angle").Output()
	testing.ContextLogf(ctx, "Lid angle using fake data is %q", string(output))

	// The second attempt should succeed since we use fake data.
	return action.Combine("recalibrate lid accelerometer operation",
		uiHelper.waitForPageToLoad("Couldn't calibrate some components", timeInSecondToLoadPage),
		uiHelper.clickToggleButton("Lid Accelerometer"),
		uiHelper.waitAndClickButton("Next", longTimeInSecondToEnableButton),
		uiHelper.waitForPageToLoad("Calibrate components", timeInSecondToLoadPage),
		uiHelper.waitAndClickButton("Next", longTimeInSecondToEnableButton),
		uiHelper.waitForPageToLoad("Calibrating components…", timeInSecondToLoadPage),
		uiHelper.waitForPageToLoad("Calibration complete", timeInSecondToLoadPage),
		uiHelper.waitAndClickButton("Next", longTimeInSecondToEnableButton),
		uiHelper.waitForPageToLoad("Finalizing repair", timeInSecondToLoadPage),
	)(ctx)
}

// CalibrateBaseGyroPageOperation handles all operations on calibrate base gyro Page.
func (uiHelper *UIHelper) CalibrateBaseGyroPageOperation(ctx context.Context) error {
	return action.Combine("calibrate base gyro operation",
		uiHelper.waitForPageToLoad("Calibrate components", timeInSecondToLoadPage),
		uiHelper.waitAndClickButton("Next", longTimeInSecondToEnableButton),
		uiHelper.waitForPageToLoad("Calibrating components…", timeInSecondToLoadPage),
		uiHelper.waitForPageToLoad("Calibration complete", timeInSecondToLoadPage),
		uiHelper.waitAndClickButton("Next", longTimeInSecondToEnableButton),
		uiHelper.waitForPageToLoad("Finalizing repair", timeInSecondToLoadPage),
	)(ctx)
}

// FinalizingRepairPageOperation handles all operations on finalizing repair Page.
func (uiHelper *UIHelper) FinalizingRepairPageOperation(ctx context.Context) error {
	return action.Combine("finalizing Repair page operation",
		uiHelper.waitForPageToLoad("Finalizing repair", timeInSecondToLoadPage),
	)(ctx)
}

// RepairCompletedPageOperation handles all operations on repair completed Page.
func (uiHelper *UIHelper) RepairCompletedPageOperation(ctx context.Context) error {
	return action.Combine("repair Completed page operation",
		uiHelper.waitForPageToLoad("Almost done!", longTimeInSecondToEnableButton),
		uiHelper.clickButton("Reboot"),
	)(ctx)
}

// VerifyLogIsSaved verifies that the log is saved in usb.
func (uiHelper *UIHelper) VerifyLogIsSaved(ctx context.Context) error {
	// Output is supposed to be something like /dev/sda1.
	usb, err := uiHelper.findUSBName(ctx)
	if err != nil {
		return errors.Wrap(err, "fail to get USB")
	}
	testing.ContextLogf(ctx, "USB is %s", usb)

	if err = uiHelper.mountUSB(ctx, usb); err != nil {
		return errors.Wrap(err, "fail to mount USB")
	}

	// Verify that we can get rma log.
	err = uiHelper.Dut.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf("ls %s/rma-*", usbTempMountDir)).Run()
	if err != nil {
		return errors.Wrap(err, "fail to find Shimless RMA log")
	}
	testing.ContextLog(ctx, "Found Shimless RMA log successfully")

	// Remove log.
	if err = uiHelper.Dut.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf("rm -r %s/rma-*", usbTempMountDir)).Run(); err != nil {
		return errors.Wrap(err, "fail to delete Shimless RMA log")
	}

	if err = uiHelper.umountUSB(ctx); err != nil {
		return errors.Wrap(err, "fail to umount USB")
	}

	return uiHelper.FirmwareHelper.Servo.SetUSBMuxState(ctx, servo.USBMuxHost)
}

// RSUPageOperation handles all operations on RSU Page.
func (uiHelper *UIHelper) RSUPageOperation(ctx context.Context) error {
	if err := action.Combine("click Challenge Code URL",
		uiHelper.waitForPageToLoad("Perform RMA Server Unlock", timeInSecondToLoadPage),
		uiHelper.clickLink("this URL"),
	)(ctx); err != nil {
		return err
	}
	url, err := uiHelper.retrieveTextByPrefix(ctx, "https")
	if err != nil {
		return err
	}
	// Extract the parameter from url.
	challengeCode, err := uiHelper.parseChallengeCode(url)
	if err != nil {
		return err
	}

	output, err := uiHelper.Dut.Conn().CommandContext(ctx, "/usr/local/bin/rma_reset", "-c", challengeCode).Output()
	if err != nil {
		return err
	}
	authCode, err := uiHelper.parseAuthCode(string(output))
	if err != nil {
		return err
	}

	return action.Combine("enter unlock code and click Next",
		uiHelper.clickButton("Done"),
		uiHelper.enterIntoTextInput(authCode, "Enter the 8-character unlock code"),
		uiHelper.clickButton("Next"),
	)(ctx)

}

// BypassFirmwareInstallation will skip firmware installation.
func (uiHelper *UIHelper) BypassFirmwareInstallation(ctx context.Context) error {
	// GoBigSleepLint: This sleep is important since we need to wait for RMAD to update state
	// file completed.
	testing.Sleep(ctx, stateFileUpdateTime)
	if _, err := uiHelper.Client.BypassFirmwareInstallation(ctx, &empty.Empty{}); err != nil {
		return err
	}

	return uiHelper.Dut.Reboot(ctx)
}

// BypassCalibration will skip calibration.
func (uiHelper *UIHelper) BypassCalibration(ctx context.Context) error {
	// GoBigSleepLint: This sleep is important since we need to wait for RMAD to update state
	// file completed.
	testing.Sleep(ctx, stateFileUpdateTime)
	_, err := uiHelper.Client.BypassCalibration(ctx, &empty.Empty{})

	return err
}

// WaitForFirmwareInstallation will trigger and wait for firmware installation.
func (uiHelper *UIHelper) WaitForFirmwareInstallation(ctx context.Context) error {
	if err := uiHelper.FirmwareHelper.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return err
	}

	// The following prints content in /dev.
	// The test will continue even if the following fails.
	if output, err := uiHelper.Dut.Conn().CommandContext(ctx, "ls", "/dev").Output(); err != nil {
		testing.ContextLogf(ctx, "Fail to ls /dev because %s", err)
	} else {
		testing.ContextLogf(ctx, "Output of ls /dev is %q", output)
	}

	testing.ContextLogf(ctx, "Sleeping %s to wait for firmware installation", firmwareInstallationTime)
	// GoBigSleepLint: Wait for firmware installation.
	return testing.Sleep(ctx, firmwareInstallationTime)
}

// SetupInitStatus setup initial status for shimless testing.
func (uiHelper *UIHelper) SetupInitStatus(ctx context.Context, enroll bool) error {
	// If error is raised, then Factory is already disabled.
	// Therefore, ignore any error.
	if err := uiHelper.changeFactoryMode("disable")(ctx); err != nil {
		testing.ContextLogf(ctx, "Fail to disable Factory Mode because %s", err)
	}

	return action.Combine("setup init status for test",
		// Reboot in |changeEnrollment| will lock CCD (after leaving factory mode),
		// so we have to call it before opening CCD.
		func(ctx context.Context) error {
			// Lab DUTs are unenrolled by default, so we only initiate the enrollment
			// process when we want to enroll the devices.
			if !enroll {
				return nil
			}
			return uiHelper.changeEnrollment(enroll)(ctx)
		},
		uiHelper.openCCDIfNotOpen(),
	)(ctx)
}

// WaitForHWWPDisableCompletePage waits the HWWP disable complete page to be loaded.
func (uiHelper *UIHelper) WaitForHWWPDisableCompletePage(ctx context.Context) error {
	return uiHelper.waitForPageToLoad("Write Protect is turned off", timeInSecondToLoadPage)(ctx)
}

// OverrideStateFile overrides state file content.
func (uiHelper *UIHelper) OverrideStateFile(ctx context.Context, content string) error {
	// Override file content.
	cmd := fmt.Sprintf(`echo '%s' > %s`, content, stateFile)
	if err := uiHelper.Dut.Conn().CommandContext(ctx, "bash", "-c", cmd).Run(); err != nil {
		return err
	}
	return uiHelper.Dut.Reboot(ctx)
}

// CleanupShimlessFiles removes the files used for running Shimless RMA tast tests.
func CleanupShimlessFiles(cleanupCtx context.Context, dut *dut.DUT) error {
	if err := dut.Conn().CommandContext(cleanupCtx, "sh", "-c", fmt.Sprintf("rm %s", stateFile)).Run(); err != nil {
		testing.ContextLogf(cleanupCtx, "Failed to delete state file because %s", err)
	}

	return dut.Reboot(cleanupCtx)
}

// CreateErrorHandler creates error handler.
func CreateErrorHandler(ctx context.Context, uiHelper **UIHelper, testname string) func(string) {
	return func(errMsg string) {
		screenshotService := graphics.NewScreenshotServiceClient((*uiHelper).RPCClient.Conn)

		if _, err := screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: testname}); err != nil {
			testing.ContextLogf(ctx, "Failed to take screenshot: %s", err)
		}

		if err := (*uiHelper).saveRmaStateFile(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to save state: %s", err)
		}
	}
}

// saveRmaStateFile saves the rmad state file to test output directory.
func (uiHelper *UIHelper) saveRmaStateFile(ctx context.Context) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		return errors.New("failed to get name of directory")
	}
	if _, err := os.Stat(dir); err != nil {
		return errors.Wrap(err, "output directory not found")
	}

	stateDir := filepath.Join(dir, "rmad_state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create directory for rmad state file")
	}

	saveStateFile := func(from, to string) error {
		rawOutput, err := readStateFile(ctx, uiHelper.Dut, from)
		if err != nil {
			return errors.Wrapf(err, "failed to read data from %s", from)
		}

		path := filepath.Join(stateDir, to)
		f, err := os.Create(path)
		if err != nil {
			return errors.Wrapf(err, "failed to create %s", to)
		}
		defer f.Close()

		if _, err := f.Write(rawOutput); err != nil {
			return errors.Wrapf(err, "failed to write data to %s", to)
		}

		testing.ContextLogf(ctx, "Saved state file to %s", path)
		return nil
	}

	if err := saveStateFile(stateFile, "state"); err != nil {
		testing.ContextLog(ctx, "Failed to save state: ", err)
	}

	return nil
}

// readStateFile reads content of rmad state file and prettify the content.
func readStateFile(ctx context.Context, dut *dut.DUT, filename string) ([]byte, error) {
	return dut.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf("jq . %s", filename)).Output()
}

func (uiHelper *UIHelper) findUSBName(ctx context.Context) (string, error) {
	output, err := uiHelper.Dut.Conn().CommandContext(ctx, "sh", "-c", "ls /dev/sd[a-z]1").Output()

	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(string(output), "\n") /** remove new line from ls output **/, nil
}

func (uiHelper *UIHelper) mountUSB(ctx context.Context, usb string) error {
	if err := uiHelper.Dut.Conn().CommandContext(ctx, "mkdir", "-p", usbTempMountDir).Run(); err != nil {
		return err
	}

	return uiHelper.Dut.Conn().CommandContext(ctx, "mount", usb, usbTempMountDir).Run()
}

func (uiHelper *UIHelper) umountUSB(ctx context.Context) error {
	return uiHelper.Dut.Conn().CommandContext(ctx, "umount", usbTempMountDir).Run()
}

func (uiHelper *UIHelper) changeEnrollment(toEnroll bool) action.Action {
	// TODO(jeffulin): Separate reboot from |changeEnrollment| so we can have more
	//                 flexibility and make action sequences more clear.
	return func(ctx context.Context) error {
		// I got the following commands from b/245415715#comment5.
		// More details can be found in the above link.
		if err := uiHelper.Dut.Conn().CommandContext(ctx, "crossystem", "clear_tpm_owner_request=1").Run(); err != nil {
			return errors.Wrap(err, "fail to clear_tpm_owner_request")
		}

		if err := uiHelper.Dut.Reboot(ctx); err != nil {
			return errors.Wrap(err, "fail to reboot after clear_tpm_owner_request")
		}

		if err := uiHelper.Dut.Conn().CommandContext(ctx, "gdbus", "wait", "--system", "--timeout", "15", "org.chromium.UserDataAuth").Run(); err != nil {
			return errors.Wrap(err, "failed to wait for D-Bus service org.chromium.UserDataAuth")
		}

		if err := uiHelper.Dut.Conn().CommandContext(ctx, "tpm_manager_client", "take_ownership").Run(); err != nil {
			return errors.Wrap(err, "fail to take ownership")
		}

		var flags string
		if toEnroll {
			flags = "--flags=0x40"
		} else {
			flags = "--flags=0"
		}

		if err := uiHelper.Dut.Conn().CommandContext(ctx, "device_management_client", "--action=set_firmware_management_parameters", flags).Run(); err != nil {
			return errors.Wrap(err, "fail to set_firmware_management_parameters")
		}
		return nil
	}
}

func (uiHelper *UIHelper) openCCDIfNotOpen() action.Action {
	return func(ctx context.Context) error {
		if val, err := uiHelper.FirmwareHelper.Servo.GetString(ctx, servo.GSCCCDLevel); err != nil {
			return err
		} else if val != servo.Open {
			if err := uiHelper.FirmwareHelper.Servo.SetString(ctx, servo.GSCTestlab, servo.Open); err != nil {
				return err
			}
		}
		return nil
	}
}

func (uiHelper *UIHelper) changeFactoryMode(status string) action.Action {
	return func(ctx context.Context) error {
		_, err := uiHelper.Dut.Conn().CommandContext(ctx, "gsctool", "-aF", status).Output()
		return err
	}
}

func (uiHelper *UIHelper) writeProtectPageOperation(radioButtonLabel string) action.Action {
	return action.Combine("write Protect page operation",
		uiHelper.waitForPageToLoad("Select how you would like to turn off Write Protect", timeInSecondToLoadPage),
		uiHelper.clickRadioButton(radioButtonLabel),
		uiHelper.waitAndClickButton("Next", timeInSecondToEnableButton),
	)
}

func (uiHelper *UIHelper) waitAndClickButton(label string, timeInSecond int32) action.Action {
	return action.Combine("wait and click button",
		func(ctx context.Context) error {
			_, err := uiHelper.Client.WaitUntilButtonEnabled(ctx, &pb.WaitUntilButtonEnabledRequest{
				Label:            label,
				DurationInSecond: timeInSecond,
			})
			if err == nil {
				testing.ContextLogf(ctx, "Found button %q enabled in %d seconds", label, timeInSecond)
			}
			return err
		},
		uiHelper.clickButton(label),
	)
}

func (uiHelper *UIHelper) clickButton(label string) action.Action {
	return func(ctx context.Context) error {
		_, err := uiHelper.Client.LeftClickButton(ctx, &pb.LeftClickButtonRequest{
			Label: label,
		})
		if err == nil {
			testing.ContextLogf(ctx, "Clicked button %q", label)
		}
		return err
	}
}

func (uiHelper *UIHelper) clickToggleButton(label string) action.Action {
	return func(ctx context.Context) error {
		_, err := uiHelper.Client.LeftClickToggleButton(ctx, &pb.LeftClickToggleButtonRequest{
			Label: label,
		})
		if err == nil {
			testing.ContextLogf(ctx, "Clicked toggle button %q", label)
		}
		return err
	}
}

func (uiHelper *UIHelper) waitForPageToLoad(title string, timeInSecond int32) action.Action {
	return func(ctx context.Context) error {
		_, err := uiHelper.Client.WaitForPageToLoad(ctx, &pb.WaitForPageToLoadRequest{
			Title:            title,
			DurationInSecond: timeInSecond,
		})
		if err == nil {
			testing.ContextLogf(ctx, "Found page titled %q in %d seconds", title, timeInSecond)
		}
		return err
	}
}

func (uiHelper *UIHelper) clickRadioButton(label string) action.Action {
	return func(ctx context.Context) error {
		_, err := uiHelper.Client.LeftClickRadioButton(ctx, &pb.LeftClickRadioButtonRequest{
			Label: label,
		})
		if err == nil {
			testing.ContextLogf(ctx, "Clicked radio button %q", label)
		}
		return err
	}
}

func (uiHelper *UIHelper) clickLink(label string) action.Action {
	return func(ctx context.Context) error {
		_, err := uiHelper.Client.LeftClickLink(ctx, &pb.LeftClickLinkRequest{
			Label: label,
		})
		if err == nil {
			testing.ContextLogf(ctx, "Clicked link %q", label)
		}
		return err
	}
}

func (uiHelper *UIHelper) retrieveTextByPrefix(ctx context.Context, prefix string) (string, error) {
	res, err := uiHelper.Client.RetrieveTextByPrefix(ctx, &pb.RetrieveTextByPrefixRequest{
		Prefix: prefix,
	})
	if err != nil {
		return "", err
	}

	return res.Value, nil
}

func (uiHelper *UIHelper) parseChallengeCode(url string) (string, error) {
	re := regexp.MustCompile("challenge=([^&]*)")
	match := re.FindStringSubmatch(url)
	if match == nil {
		return "", errors.New("fail to get Challenge Code")
	}
	return match[1], nil
}

func (uiHelper *UIHelper) parseAuthCode(raw string) (string, error) {
	re := regexp.MustCompile(`Authcode:\s+([a-zA-Z0-9]*)`)
	match := re.FindStringSubmatch(raw)
	if match == nil {
		return "", errors.New("fail to get Auth Code")
	}

	return match[1], nil
}

func (uiHelper *UIHelper) enterIntoTextInput(content, textInputName string) action.Action {
	return func(ctx context.Context) error {
		_, err := uiHelper.Client.EnterIntoTextInput(ctx, &pb.EnterIntoTextInputRequest{
			TextInputName: textInputName,
			Content:       content,
		})
		if err == nil {
			testing.ContextLogf(ctx, "Inputted %q in field %q", content, textInputName)
		}
		return err
	}
}

func (uiHelper *UIHelper) disconnectBatteryByCr50() action.Action {
	return func(ctx context.Context) error {
		return servoutil.SetBatteryState(ctx, uiHelper.FirmwareHelper, servoutil.BatteryStateOff)
	}
}

func (uiHelper *UIHelper) flattenDutWithFakeSensorData(ctx context.Context) error {
	// The following data is provided by ShimlessRMA team (genechang@).
	return uiHelper.Dut.Conn().CommandContext(ctx, "ectool", "motionsense", "spoof", "0", "1", "0", "0", "16373").Run()
}

func (uiHelper *UIHelper) resetToUseRealSensorData(ctx context.Context) error {
	testing.ContextLog(ctx, "Reset Accel to use real data")
	if err := uiHelper.Dut.Conn().CommandContext(ctx, "ectool", "motionsense", "spoof", "0", "0").Run(); err != nil {
		return err
	}

	output, _ := uiHelper.Dut.Conn().CommandContext(ctx, "ectool", "motionsense", "lid_angle").Output()
	testing.ContextLogf(ctx, "Lid angle after reset is %q", string(output))

	return nil
}

func getRmadStateJSON(ctx context.Context, dut *dut.DUT) (map[RmadStateField]interface{}, error) {
	data, err := readStateFile(ctx, dut, stateFile)
	if err != nil {
		return nil, errors.Wrap(err, "fail to read state file")
	}

	var state map[RmadStateField]interface{}
	err = json.Unmarshal(data, &state)
	if err != nil {
		return nil, errors.Wrap(err, "fail to unmarshal state file")
	}

	return state, nil
}

// PollStateField asserts that the value of a given field in the rmad state file matches an expected value.
func PollStateField(ctx context.Context, s *testing.State, key RmadStateField, expectedValue interface{}, timeout time.Duration) error {
	firmwareHelper := s.FixtValue().(*fixture.Value).Helper
	dut := firmwareHelper.DUT

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		s.Logf("Polling %s, expecting %v", key, expectedValue)

		if err := firmwareHelper.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "fail to connect")
		}

		state, err := getRmadStateJSON(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "fail to get rmad state")
		}

		if state[key] != expectedValue {
			return errors.New("Device has not rebooted yet")
		}

		s.Logf("Asserted %s to be %v", key, expectedValue)

		return nil
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrapf(err, "fail to wait for %s being %v: ", key, expectedValue)
	}

	return nil
}
