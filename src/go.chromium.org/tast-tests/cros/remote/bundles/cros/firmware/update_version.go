// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

type updateVersionTc struct {
	makekeyFile string
	commonFile  string
}

var (
	fwDataKeyVerMakekeyFile = "fwDataKeyVer/make_keys.sh"
	fwDataKeyVerCommonFile  = "fwDataKeyVer/common.sh"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateVersion,
		Desc: "Update the firmware data key version with autoupdate mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Vars:         []string{"firmware.skipFlashUSB"},
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_usb"},
		Timeout:      120 * time.Minute,
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.TPMService"},
		SoftwareDeps: []string{"flashrom"},
		Params: []testing.Param{
			{
				Name:      "firmware_data_key_version",
				Fixture:   fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				ExtraData: []string{fwDataKeyVerMakekeyFile, fwDataKeyVerCommonFile},
				Val: &updateVersionTc{
					makekeyFile: fwDataKeyVerMakekeyFile,
					commonFile:  fwDataKeyVerCommonFile,
				},
			},
		},
	})
}

func UpdateVersion(ctx context.Context, s *testing.State) {
	const (
		tempDir = "/usr/local/tmp/faft"
	)
	var (
		backupManager = s.FixtValue().(*fixture.Value).BackupManager
		h             = s.FixtValue().(*fixture.Value).Helper
		workDir       = filepath.Join(tempDir, "autest")
		apBinary      = filepath.Join(workDir, "bios.bin")
		keysDir       = filepath.Join(workDir, "keys")
		bootOk        = "/usr/sbin/chromeos-setgoodfirmware"
		tc            = s.Param().(*updateVersionTc)
	)

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Requiring RPC client")
	}

	// Check if the DUT's active RW firmware is RWA. If not, reboot to RWA.
	if currentActRW, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct); err != nil {
		s.Fatal("Failed to get crossystem mainfw_act: ", err)
	} else if currentActRW != "A" {
		if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamFWTryNext, "A"); err != nil {
			s.Fatal("Failed to set crossystem fw_try_next: ", err)
		}
		if err := rebootDUTAndRequireRPCClient(ctx, h); err != nil {
			s.Fatal("Failed to reboot DUT: ", err)
		}
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()

	fs := dutfs.NewClient(h.RPCClient.Conn)
	if exist, err := fs.Exists(ctx, tempDir); err != nil {
		s.Fatal("Failed to check if temp dir is exist: ", err)
	} else if exist {
		// If temp dir is exist, remove it first.
		if err := fs.RemoveAll(ctx, tempDir); err != nil {
			s.Fatal("Failed to remove temp dir: ", err)
		}
	}
	if err := fs.MkDir(ctx, tempDir, 0777); err != nil {
		s.Fatal("Failed to make the temp directory: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Cleanup directories")
		fs := dutfs.NewClient(h.RPCClient.Conn)
		if err := fs.RemoveAll(ctx, tempDir); err != nil {
			s.Fatal("Failed to remove temp dir: ", err)
		}
	}(cleanupCtx)

	initFwid, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid)
	if err != nil {
		s.Fatal("Failed to get crossystem fwid: ", err)
	}

	skipFlashUSB := false
	if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
		skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
		}
	}
	cs := s.CloudStorage()
	if skipFlashUSB {
		cs = nil
	}
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	s.Log("Copy the AP firmware binary to the DUT")
	if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, apBinary); err != nil {
		s.Fatal("Failed to copy AP firmware binary to DUT: ", err)
	}

	initTpmDatakeyVer, err := getTPMDataKeyVer(ctx, h)
	if err != nil {
		s.Fatal("Failed to get TPM data key version: ", err)
	}
	initFwDataKeyVer, err := h.GetCurrentFwDataKeyVersion(ctx, bios.FWSignAImageSection)
	if err != nil {
		s.Fatal("Failed to get firmware data key version: ", err)
	}
	s.Logf("initTpmDatakeyVer is %v, initFwDataKeyVer is %v", initTpmDatakeyVer, initFwDataKeyVer)

	newDataKeyVer := initFwDataKeyVer + 1
	s.Logf("Firmware version will update to version %s", fmt.Sprint(newDataKeyVer))

	s.Log("Prepare the key files that are going to be resigned")
	if err := prepareKeyfile(ctx, h, keysDir); err != nil {
		s.Fatal("Failed to prepare the key files: ", err)
	}

	// Send the shell scripts used to resign the keys from the host to the DUT.
	if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{s.DataPath(tc.makekeyFile): filepath.Join(workDir, tc.makekeyFile)}, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send %v to DUT: %v", tc.makekeyFile, err)
	}
	if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{s.DataPath(tc.commonFile): filepath.Join(workDir, tc.commonFile)}, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send %v to DUT: %v", tc.commonFile, err)
	}

	// Generate the files required for signing by executing the file make_keys.sh.
	if err := h.DUT.Conn().CommandContext(ctx, "/bin/bash", filepath.Join(workDir, tc.makekeyFile), fmt.Sprint(newDataKeyVer)).Run(ssh.DumpLogOnError); err != nil {
		s.Fatalf("Failed to execute %v on DUT: %v", filepath.Join(workDir, tc.makekeyFile), err)
	}

	// Sign the BIOS binary file to generate a new binary file (output.bin) with an updated data key version.
	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		s.Fatal("Failed to create futility instance: ", err)
	}

	signOpts := futility.
		NewSignBIOSOptions(apBinary).
		WithOutputFile(filepath.Join(workDir, "output.bin")).
		WithSignPrivatePath(filepath.Join(keysDir, "firmware_data_key.vbprivk")).
		WithKeyBlockPath(filepath.Join(keysDir, "firmware.keyblock")).
		WithKernelKeyPath(filepath.Join(keysDir, "kernel_subkey.vbpubk")).
		WithVersion(1)

	if _, err := futilityInstance.SignBIOS(ctx, signOpts); err != nil {
		s.Fatal("Failed to use futility to autoupdate inactive firmware (RWB): ", err)
	}

	if err := fwTriesChecker(ctx, h, "A", 0); err != nil {
		s.Fatal("Failed to check firmware tries: ", err)
	}

	// It should update the inactive RW firmware (RWB) to new firmware data key version.
	s.Log("Update the firmware with autoUpdate mode")
	autoupdateOpts := futility.
		NewUpdateOptions(filepath.Join(workDir, "output.bin")).
		WithMode(futility.UpdateModeAutoUpdate).
		WithWriteProtection(futility.WriteProtectionEnable).
		WithHostOnly(true)

	if _, err := futilityInstance.Update(ctx, autoupdateOpts); err != nil {
		s.Fatal("Failed to use futility to autoupdate inactive firmware (RWB): ", err)
	}
	defer func(ctx context.Context) {
		// Ensure tpm data key version is same as original one.
		currentTpmDatakeyVer, err := getTPMDataKeyVer(ctx, h)
		if err != nil {
			s.Error("Failed to get TPM data key version: ", err)
		}

		if currentTpmDatakeyVer != initTpmDatakeyVer {
			s.Log("Reset TPM and reboot DUT")
			if err := resetTpmAndReboot(ctx, h); err != nil {
				s.Fatal("Failed to reset TPM: ", err)
			}
		}
	}(cleanupCtx)

	if err := rebootDUTAndRequireRPCClient(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	// Mark RWB firmware is a good firmware to finish the firmware autoUpdate procedure.
	if err := h.DUT.Conn().CommandContext(ctx, bootOk).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to run the boot_ok script on DUT: ", err)
	}

	if err := fwTriesChecker(ctx, h, "B", 0); err != nil {
		s.Fatal("Failed to check firmware tries: ", err)
	}

	if err := rebootDUTAndRequireRPCClient(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	if err := fwTriesChecker(ctx, h, "B", 0); err != nil {
		s.Fatal("Failed to check firmware tries: ", err)
	}

	if err := checkDataKeyVer(ctx, h, newDataKeyVer, bios.FWSignBImageSection); err != nil {
		s.Fatal("Failed to check the data key version: ", err)
	}

	s.Log("Rollback the DUT with recovery mode using original bios binary file")

	if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, apBinary); err != nil {
		s.Fatal("Failed to copy AP firmware binary to DUT: ", err)
	}

	recoveryOpts := futility.
		NewUpdateOptions(apBinary).
		WithMode(futility.UpdateModeRecovery).
		WithWriteProtection(futility.WriteProtectionEnable).
		WithHostOnly(true).
		WithForce(true)

	if _, err := futilityInstance.Update(ctx, recoveryOpts); err != nil {
		s.Fatal("Failed to use futility update to restore the firmware: ", err)
	}

	// Reset the TPM to avoid encountering the 'RW firmware key version rollback detect' issue after reboot.
	s.Log("Reset TPM and reboot DUT")
	if err := resetTpmAndReboot(ctx, h); err != nil {
		s.Fatal("Failed to reset TPM and reboot DUT: ", err)
	}

	// Get the firmware ID from 'crossystem fwid' and check if it is the same as the original one.
	if currentFwid, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid); err != nil {
		s.Fatal("Failed to get crossystem fwid: ", err)
	} else if currentFwid != initFwid {
		s.Fatalf("Current fwid (%v) is not equal to the original fwid (%v)", currentFwid, initFwid)
	}

	if err := fwTriesChecker(ctx, h, "B", 0); err != nil {
		s.Fatal("Failed to check firmware tries: ", err)
	}

	// Check if the firmware data key version is rollback to original one.
	if err := checkDataKeyVer(ctx, h, initFwDataKeyVer, bios.FWSignBImageSection); err != nil {
		s.Fatal("Failed to check the data key version: ", err)
	}
}

// getTPMDataKeyVer retrieves the data key version from TPM.
func getTPMDataKeyVer(ctx context.Context, h *firmware.Helper) (uint16, error) {
	tpmFwKeyVersion, err := h.ReadTPMC(ctx, "read", fmt.Sprintf("0x%x", fwCommon.TpmFirmwareNvIndex), fmt.Sprintf("0x%x", fwCommon.TpmFirmwareNvSize))
	if err != nil {
		return 0, errors.Wrap(err, "failed to read tpm data key version")
	}

	// Check that there are no extra lines of output beyond the expected single line of TPM data.
	lines := strings.Split(tpmFwKeyVersion, "\n")
	if len(lines[1]) != 0 {
		return 0, errors.Errorf("unexpected additional data beyond the first line of TPM output: %s", tpmFwKeyVersion)
	}

	dataBytes := strings.Split(lines[0], " ")

	highByte, err := strconv.ParseUint(dataBytes[5], 10, 8)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert high byte %v", dataBytes[5])
	}
	lowByte, err := strconv.ParseUint(dataBytes[4], 10, 8)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert low byte %v", dataBytes[4])
	}

	return uint16(highByte<<8 + lowByte), nil
}

// fwTriesChecker check if the specific 'crossystem' outputs are as expected.
func fwTriesChecker(ctx context.Context, h *firmware.Helper, expectedMainFwAct string, expectedTryCount int) error {
	expectedMainFwAct = strings.ToUpper(expectedMainFwAct)
	expectedCrossParam := map[reporters.CrossystemParam]string{
		reporters.CrossystemParamMainfwAct: expectedMainFwAct,
	}
	expectedCrossParam[reporters.CrossystemParamFWTryCount] = fmt.Sprint(expectedTryCount)

	for k, v := range expectedCrossParam {
		actureValue, err := h.Reporter.CrossystemParam(ctx, k)
		if err != nil {
			return errors.Wrapf(err, "failed to get crossystem %v", k)
		}
		if actureValue != v {
			return errors.Errorf("expected %v but got %v", v, actureValue)
		}
	}

	return nil
}

// rebootDUTAndRequireRPCClient reboot the DUT and require the BiosServiceClient.
func rebootDUTAndRequireRPCClient(ctx context.Context, h *firmware.Helper) error {
	h.CloseRPCConnection(ctx)
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrap(err, "failed to reset DUT")
	}

	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "failed to require RPC client")
	}
	return nil
}

// checkDataKeyVer checks if the data key version
// obtained from the current firmware section and TPM are both as expected.
func checkDataKeyVer(ctx context.Context, h *firmware.Helper, expectedVer uint16, sec bios.ImageSection) error {
	testing.ContextLog(ctx, "Check the datakey version of TPM and firmware")
	// Get the data key version from current firmware section.
	actualFwDataKeyVer, err := h.GetCurrentFwDataKeyVersion(ctx, sec)
	if err != nil {
		return errors.Wrap(err, "failed to get firmware data key version")
	}
	// Get the data key version from TPM.
	actualTpmDataKeyVer, err := getTPMDataKeyVer(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get TPM data key version")
	}

	if actualFwDataKeyVer != expectedVer || actualTpmDataKeyVer != expectedVer {
		return errors.Errorf("Data key version should be %v, but got (fwver, tpm_fwver) = (%v, %v)", expectedVer, actualFwDataKeyVer, actualTpmDataKeyVer)
	}
	testing.ContextLog(ctx, "Update success, now datakey version is ", actualFwDataKeyVer)
	return nil
}

// resetTpmAndReboot reset the TPM's data key version and reboot.
func resetTpmAndReboot(ctx context.Context, h *firmware.Helper) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create new boot mode switcher")
	}
	testing.ContextLog(ctx, "Rebooting the DUT to recovery screen")
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to reboot to recovery screen")
	}

	testing.ContextLog(ctx, "Checking if DUT boots from the USB")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
		return errors.Wrap(err, "failed to boot from the USB")
	}

	testing.ContextLog(ctx, "Running TPM recovery command - chromeos-tpm-recovery to clear tpm data")
	cmd := h.DUT.Conn().CommandContext(ctx, "chromeos-tpm-recovery")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to run chromeos-tpm-recovery: %v", string(out))
	}
	testing.ContextLog(ctx, "TPM recovery command output : ", string(out))

	h.CloseRPCConnection(ctx)
	testing.ContextLog(ctx, "Reboot the DUT")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		return errors.Wrap(err, "faild to reset DUT")
	}

	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to dut")
	}

	return nil
}

// prepareKeyfile prepare the key files that are going to be resigned.
func prepareKeyfile(ctx context.Context, h *firmware.Helper, keysDir string) error {
	fs := dutfs.NewClient(h.RPCClient.Conn)
	if err := fs.RemoveAll(ctx, keysDir); err != nil {
		return errors.Wrapf(err, "failed to remove the dir %v", keysDir)
	}
	if err := fs.CopyDir(ctx, "/usr/share/vboot/devkeys", keysDir); err != nil {
		return errors.Wrapf(err, "failed to copy the key files from %v to %v", "/usr/share/vboot/devkeys", keysDir)
	}
	return nil
}
