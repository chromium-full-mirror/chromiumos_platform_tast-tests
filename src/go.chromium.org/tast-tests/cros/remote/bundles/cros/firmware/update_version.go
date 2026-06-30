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
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

type keyVersType int

const (
	fwDataKeyVer keyVersType = iota
	kernelSubkeyVer
	fwVer
)

type updateVersionTc struct {
	makekeyFile         string
	commonFile          string
	keyVersion          keyVersType
	tpmNvRAMHighByteIdx int
	tpmNvRAMLowByteIdx  int
}

var (
	fwDataKeyVerMakekeyFile    = "fwDataKeyVer/make_keys.sh"
	fwDataKeyVerCommonFile     = "fwDataKeyVer/common.sh"
	kernelSubkeyVerMakekeyFile = "kernelSubkeyVer/make_keys.sh"
	kernelSubkeyVerCommonFile  = "kernelSubkeyVer/common.sh"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateVersion,
		Desc: "Verify if the key version matches the expectation after autoupdate mode",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Timeout:      120 * time.Minute,
		ServiceDeps:  []string{"tast.cros.firmware.TPMService"},
		Params: []testing.Param{
			{
				Name:             "firmware_data_key_version",
				ExtraTestBedDeps: []string{tbdep.ServoUSBState("NORMAL")},
				Fixture:          fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				ExtraData:        []string{fwDataKeyVerMakekeyFile, fwDataKeyVerCommonFile},
				Val: &updateVersionTc{
					makekeyFile:         fwDataKeyVerMakekeyFile,
					commonFile:          fwDataKeyVerCommonFile,
					keyVersion:          fwDataKeyVer,
					tpmNvRAMHighByteIdx: 5,
					tpmNvRAMLowByteIdx:  4,
				},
			},
			{
				Name:      "kernel_subkey_version",
				Fixture:   fixture.BootModeFixtureWithAPBackup(fixture.DevModeGBB),
				ExtraData: []string{kernelSubkeyVerMakekeyFile, kernelSubkeyVerCommonFile},
				Val: &updateVersionTc{
					makekeyFile: kernelSubkeyVerMakekeyFile,
					commonFile:  kernelSubkeyVerCommonFile,
					keyVersion:  kernelSubkeyVer,
				},
			},
			{
				Name:             "firmware_version",
				ExtraTestBedDeps: []string{tbdep.ServoUSBState("NORMAL")},
				ExtraAttr:        []string{"firmware_smoke"},
				Fixture:          fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val: &updateVersionTc{
					keyVersion:          fwVer,
					tpmNvRAMHighByteIdx: 3,
					tpmNvRAMLowByteIdx:  2,
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
		pv            = s.FixtValue().(*fixture.Value)
		backupManager = pv.BackupManager
		h             = pv.Helper
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
		s.Fatal("Failed to require RPC client: ", err)
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
	if err := fwTriesChecker(ctx, h, "A", 0); err != nil {
		s.Fatal("Failed to check firmware tries: ", err)
	}

	fs := dutfs.NewClient(h.RPCClient.Conn)
	if exist, err := fs.Exists(ctx, tempDir); err != nil {
		s.Fatal("Failed to check if temp dir is exist: ", err)
	} else if exist {
		if err := fs.RemoveAll(ctx, tempDir); err != nil {
			s.Fatal("Failed to remove temp dir: ", err)
		}
	}

	s.Log("Creating temp directories")
	if err := fs.MkDir(ctx, tempDir, 0777); err != nil {
		s.Fatal("Failed to make the temp directory: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Make sure DUT is connected before cleanup")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to connect to the DUT: ", err)
		}

		if err := h.RequireRPCClient(ctx); err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		s.Log("Cleaning up temp directories")
		fs := dutfs.NewClient(h.RPCClient.Conn)
		if err := fs.RemoveAll(ctx, tempDir); err != nil {
			s.Fatal("Failed to remove temp dir: ", err)
		}
	}(ctx)

	var err error
	var initTpmNvRAM uint16
	if tc.keyVersion == fwDataKeyVer || tc.keyVersion == fwVer {
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

		initTpmNvRAM, err = getTPMNvRAM(ctx, h, tc)
		if err != nil {
			s.Fatal("Failed to get TPM version: ", err)
		}
		s.Logf("Initial TPM version is %s", fmt.Sprint(initTpmNvRAM))
	}

	s.Log("Copying the AP firmware binary to the DUT")
	if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, apBinary); err != nil {
		s.Fatal("Failed to copy AP firmware binary to DUT: ", err)
	}

	var keyVerOpts firmware.KeyVersOptions
	switch tc.keyVersion {
	case fwDataKeyVer:
		keyVerOpts = firmware.KeyVersOptions{
			Section: bios.FWSignAImageSection,
			Type:    firmware.FwDataKey,
		}
	case kernelSubkeyVer:
		keyVerOpts = firmware.KeyVersOptions{
			Section: bios.FWSignAImageSection,
			Type:    firmware.KernelSubkey,
		}
	case fwVer:
		keyVerOpts = firmware.KeyVersOptions{
			Section: bios.FWSignAImageSection,
			Type:    firmware.FWVersion,
		}
	default:
		s.Fatal("Invalid key version: ", tc.keyVersion)
	}

	// Get RWA key version to check if the RWA and RWB key versions are the same.
	initRWAKeyVer, err := h.GetCurrentKeyVersion(ctx, keyVerOpts)
	if err != nil {
		s.Fatal("Failed to get current key version of RWA: ", err)
	}

	// Get RWB key version
	keyVerOpts.Section = bios.FWSignBImageSection
	initRWBKeyVer, err := h.GetCurrentKeyVersion(ctx, keyVerOpts)
	if err != nil {
		s.Fatal("Failed to get current key version of RWB: ", err)
	} else if initRWBKeyVer != initRWAKeyVer {
		s.Fatalf("The key version of RWA (%v) is not equal to RWB (%v) at the beginning, expected RWA and RWB have same key version", initRWAKeyVer, initRWBKeyVer)
	}
	s.Logf("Initial key version is %s", fmt.Sprint(initRWBKeyVer))

	newKeyVer := initRWBKeyVer + 1

	s.Logf("Firmware version will update to version %s", fmt.Sprint(newKeyVer))
	s.Log("Preparing the key files that are going to be resigned")
	if err := h.PrepareKeysWithScript(ctx, firmware.MakeKeysOption{
		VersionToSign: fmt.Sprint(newKeyVer),
		// Copies original key files from the DUT to the KeysDir
		KeysDir:        keysDir,
		MakeKeyFileDir: tc.makekeyFile,
		CommonFileDir:  tc.commonFile,
		//  Absolute paths for the shell scripts in firmware/data
		ShellScript: s.DataPaths(),
	}); err != nil {
		s.Fatal("Failed to prepare the key files: ", err)
	}

	resignFWVersion := int(newKeyVer)
	if tc.makekeyFile != "" && tc.commonFile != "" {
		resignFWVersion = 1
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
		WithVersion(resignFWVersion)

	if _, err := futilityInstance.SignBIOS(ctx, signOpts); err != nil {
		s.Fatal("Failed to use futility to autoupdate inactive firmware (RWB): ", err)
	}

	// It should update the inactive RW firmware (RWB) to new key version.
	s.Log("Updating the firmware with autoupdate mode")
	autoupdateOpts := futility.
		NewUpdateOptions(filepath.Join(workDir, "output.bin")).
		WithMode(futility.UpdateModeAutoUpdate).
		WithWriteProtection(futility.WriteProtectionEnable).
		WithHostOnly(true)

	if _, err := futilityInstance.Update(ctx, autoupdateOpts); err != nil {
		s.Fatal("Failed to use futility to autoupdate inactive firmware (RWB): ", err)
	}

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)
	fwidAfterAutoUpdate := new(string)

	defer func(ctx context.Context, fwidAfterAutoUpdate *string) {
		s.Log("Make sure DUT is connected before cleanup")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to connect to the DUT: ", err)
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

		if tc.keyVersion == fwDataKeyVer || tc.keyVersion == fwVer {
			// Reset the TPM to avoid encountering the 'RW firmware key version rollback detect' issue after reboot.
			s.Log("Resetting TPM and rebooting DUT")
			if err := resetTpmAndReboot(ctx, pv, &state); err != nil {
				s.Error("Failed to reset TPM and reboot DUT: ", err)
			}
			if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
				if err := h.SetDUTPower(ctx, true); err != nil {
					s.Fatal("Failed to connect charger: ", err)
				}
				state.IsServoChargerConnected = true
				waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
				defer cancelWaitConnect()
				if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
					s.Fatal("Failed to reconnect to the DUT: ", err)
				}
			}
		} else if tc.keyVersion == kernelSubkeyVer {
			if err := h.RebootWithSSHCommand(ctx, pv.BootMode); err != nil {
				s.Fatal("Failed to reboot with VT2 command: ", err)
			}
		}

		// If there were any errors before, we do not need further verification.
		if s.HasError() {
			return
		}

		// Check if the FWID after rollback is the same as the one obtained after the autoupdate.
		if fwidAfterRollback, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid); err != nil {
			s.Fatal("Failed to get current fwid after rollback: ", err)
		} else if fwidAfterRollback != *fwidAfterAutoUpdate {
			s.Fatalf("The fwid (%v) after rollback is not equal to the fwid (%v) after autoupdate", fwidAfterRollback, fwidAfterAutoUpdate)
		}

		if err := fwTriesChecker(ctx, h, "B", 0); err != nil {
			s.Fatal("Failed to check firmware tries: ", err)
		}

		// Check if the key version is rollback to original one.
		if err := checkKeyVer(ctx, h, initRWAKeyVer, keyVerOpts, tc); err != nil {
			s.Fatal("Failed to check the key version: ", err)
		}
	}(ctx, fwidAfterAutoUpdate)

	if err := rebootDUTAndRequireRPCClient(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	if fwidTmp, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid); err != nil {
		s.Fatal("Failed to get current fwid after autoupdate: ", err)
	} else {
		*fwidAfterAutoUpdate = fwidTmp
	}

	// Mark RWB firmware is a good firmware to finish the firmware autoUpdate procedure.
	if err := h.DUT.Conn().CommandContext(ctx, bootOk).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to run the boot_ok script on DUT: ", err)
	}

	if err := fwTriesChecker(ctx, h, "B", 0); err != nil {
		s.Fatal("Failed to check firmware tries: ", err)
	}

	if tc.keyVersion == fwDataKeyVer || tc.keyVersion == fwVer {
		if err := rebootDUTAndRequireRPCClient(ctx, h); err != nil {
			s.Fatal("Failed to reboot DUT: ", err)
		}

		if err := fwTriesChecker(ctx, h, "B", 0); err != nil {
			s.Fatal("Failed to check firmware tries: ", err)
		}
	}

	// Verify that the RWB firmware is updated after the auto-update.
	if err := checkKeyVer(ctx, h, newKeyVer, keyVerOpts, tc); err != nil {
		s.Fatal("Failed to check the key version: ", err)
	}
}

// getTPMNvRAM retrieves the NvRam content from TPM based on its high and low byte.
func getTPMNvRAM(ctx context.Context, h *firmware.Helper, tc *updateVersionTc) (uint16, error) {
	tpmFwKeyVersion, err := h.ReadTPMC(ctx, "read", fmt.Sprintf("0x%x", fwCommon.TpmFirmwareNvIndex), fmt.Sprintf("0x%x", fwCommon.TpmFirmwareNvSize))
	if err != nil {
		return 0, errors.Wrap(err, "failed to read tpm NvRam content")
	}

	// Check that there are no extra lines of output beyond the expected single line of TPM data.
	lines := strings.Split(tpmFwKeyVersion, "\n")
	if len(lines[1]) != 0 {
		return 0, errors.Errorf("unexpected additional data beyond the first line of TPM output: %s", tpmFwKeyVersion)
	}

	dataBytes := strings.Split(lines[0], " ")

	highByte, err := strconv.ParseUint(dataBytes[tc.tpmNvRAMHighByteIdx], 10, 8)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert high byte %v", dataBytes[tc.tpmNvRAMHighByteIdx])
	}
	lowByte, err := strconv.ParseUint(dataBytes[tc.tpmNvRAMLowByteIdx], 10, 8)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert low byte %v", dataBytes[tc.tpmNvRAMLowByteIdx])
	}

	return uint16(highByte<<8 + lowByte), nil
}

// fwTriesChecker checks if the specific 'crossystem' outputs are as expected.
func fwTriesChecker(ctx context.Context, h *firmware.Helper, expectedMainFwAct string, expectedTryCount int) error {
	expectedCrossParam := map[reporters.CrossystemParam]string{
		reporters.CrossystemParamMainfwAct:  strings.ToUpper(expectedMainFwAct),
		reporters.CrossystemParamFWTryCount: fmt.Sprint(expectedTryCount),
	}

	if matched, err := h.Reporter.CrossystemChecker(ctx, expectedCrossParam); err != nil {
		return errors.Wrap(err, "failed to verify crossystem params")
	} else if !matched {
		return errors.New("failed to verify fw_try_count and mainfw_act are not as expected")
	}
	return nil
}

// rebootDUTAndRequireRPCClient reboot the DUT and require the BiosServiceClient.
func rebootDUTAndRequireRPCClient(ctx context.Context, h *firmware.Helper) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "creating mode switcher")
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "failed to require RPC client")
	}
	return nil
}

// checkKeyVer checks if the key version obtained from the current firmware section is as expected.
// If the key version type is firmware data key, it will also check the TPM data key version.
func checkKeyVer(ctx context.Context, h *firmware.Helper, expectedVer uint16, keyVerOpts firmware.KeyVersOptions, tc *updateVersionTc) error {
	// Get the key version from current firmware section.
	actualKeyVer, err := h.GetCurrentKeyVersion(ctx, keyVerOpts)
	if err != nil {
		return errors.Wrap(err, "failed to get actual key version")
	}

	switch keyVerOpts.Type {
	case firmware.FwDataKey:
		fallthrough
	case firmware.FWVersion:
		actualTpmVer, err := getTPMNvRAM(ctx, h, tc)
		if err != nil {
			return errors.Wrap(err, "failed to get TPM version")
		}
		if actualKeyVer != expectedVer || actualTpmVer != expectedVer {
			return errors.Errorf("expected version should be %v, but got (fwver, tpm_fwver) = (%v, %v)", expectedVer, actualKeyVer, actualTpmVer)
		}
	case firmware.KernelSubkey:
		if actualKeyVer != expectedVer {
			return errors.Errorf("kernel subkey version should be %v, but got %v", expectedVer, actualKeyVer)
		}
	default:
		return errors.New("invalid key version type")
	}

	testing.ContextLog(ctx, "Update success, now key version is ", actualKeyVer)
	return nil
}

// resetTpmAndReboot resets the TPM's data key version and reboots.
func resetTpmAndReboot(ctx context.Context, pv *fixture.Value, state *firmware.CheckAndSetServoCharger) error {
	h := pv.Helper
	testing.ContextLog(ctx, "Rebooting the DUT to recovery screen")
	if err := h.BootToRecoveryMode(ctx, state, false); err != nil {
		return errors.Wrap(err, "failed to boot to recovery mode")
	}

	testing.ContextLog(ctx, "Running TPM recovery command - chromeos-tpm-recovery to clear tpm data")
	cmd := h.DUT.Conn().CommandContext(ctx, "chromeos-tpm-recovery")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to run chromeos-tpm-recovery: %v", string(out))
	}
	testing.ContextLog(ctx, "TPM recovery command output : ", string(out))

	if err := h.RebootWithSSHCommand(ctx, pv.BootMode); err != nil {
		return errors.Wrap(err, "failed to reboot with VT2 command")
	}
	return nil
}
