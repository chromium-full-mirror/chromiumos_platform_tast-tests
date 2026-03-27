// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import (
	"context"
	"strconv"
	"time"

	fp "go.chromium.org/tast-tests/cros/common/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// FWImageType is the type of firmware (RO or RW).
type FWImageType string

// These are the possible values of FWImageType.
const (
	ImageTypeRO FWImageType = "RO"
	ImageTypeRW FWImageType = "RW"
)

const (
	ectoolROVersion = "RO version"
	ectoolRWVersion = "RW version"
)

// EctoolCommand constructs an "ectool" command for the FPMCU.
func EctoolCommand(ctx context.Context, d *dut.DUT, args ...string) *ssh.Cmd {
	cmd := firmware.NewECTool(d, firmware.ECToolNameFingerprint).Command(ctx, args...)
	testing.ContextLogf(ctx, "Running command: %s", shutil.EscapeSlice(cmd.Args))
	return cmd
}

// UnmarshalEctoolFlags unmarshals part of the ectool output into a flags.
func UnmarshalEctoolFlags(data string) (uint32, error) {
	flags, err := strconv.ParseUint(data, 0, 32)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert ectool flags (%q) to unsigned integer", data)
	}
	return uint32(flags), nil
}

// SecretInitializedStatus represents the state of the secret initialization.
type SecretInitializedStatus int

const (
	// SecretInitializedUnknown means the secret status was not reported.
	SecretInitializedUnknown SecretInitializedStatus = iota
	// SecretInitializedFalse means the secret is not initialized.
	SecretInitializedFalse
	// SecretInitializedTrue means the secret is initialized.
	SecretInitializedTrue
)

// RollbackState is the state of the anti-rollback block.
type RollbackState struct {
	BlockID           int
	MinVersion        int
	RWVersion         int
	SecretInitialized SecretInitializedStatus
}

// UnmarshalerEctool unmarshals part of ectool's output into a RollbackState.
func (r *RollbackState) UnmarshalerEctool(data []byte) error {
	rollbackInfoMap := fp.ParseColonDelimitedOutput(string(data))

	var state RollbackState
	blockID, err := strconv.Atoi(rollbackInfoMap["Rollback block id"])
	if err != nil {
		return errors.Wrap(err, "failed to convert rollback block id")
	}
	state.BlockID = blockID

	minVersion, err := strconv.Atoi(rollbackInfoMap["Rollback min version"])
	if err != nil {
		return errors.Wrap(err, "failed to convert rollback min version")
	}
	state.MinVersion = minVersion

	rwVersion, err := strconv.Atoi(rollbackInfoMap["RW rollback version"])
	if err != nil {
		return errors.Wrap(err, "failed to convert RW rollback version")
	}
	state.RWVersion = rwVersion

	if secretInitializedStr, ok := rollbackInfoMap["Secret initialized"]; ok {
		secretInitialized, err := strconv.Atoi(secretInitializedStr)
		if err != nil {
			return errors.Wrap(err, "failed to convert secret initialized")
		}
		if secretInitialized != 0 {
			state.SecretInitialized = SecretInitializedTrue
		} else {
			state.SecretInitialized = SecretInitializedFalse
		}
	} else {
		state.SecretInitialized = SecretInitializedUnknown
	}

	*r = state
	return nil
}

// IsEntropySet checks that entropy has already been set, based on the rollback state.
//
// If the secret state is unknown, it falls back to checking the block ID.
// If the block ID is greater than 0, there is a very good chance that entropy
// has been added. This is the same way that biod/bio_wash checks if entropy has
// been set.
func (r *RollbackState) IsEntropySet(ctx context.Context) bool {
	if r.SecretInitialized != SecretInitializedUnknown {
		return r.SecretInitialized == SecretInitializedTrue
	}
	testing.ContextLog(ctx, "It's not possible to reliably determine whether the entropy is set. Falling back to checking block id")
	return r.BlockID > 0
}

// IsAntiRollbackVersionCorrect checks if current RW rollback version matches the minimal rollback version.
func (r *RollbackState) IsAntiRollbackVersionCorrect() bool {
	return r.RWVersion == r.MinVersion
}

// IsSecretInitializationStatusSupported provides information whether the firmware
// supports secret initialization status reporting.
func (r *RollbackState) IsSecretInitializationStatusSupported() bool {
	return r.SecretInitialized != SecretInitializedUnknown
}

// RollbackInfo returns the rollbackinfo of the fingerprint MCU.
func RollbackInfo(ctx context.Context, d *dut.DUT) (RollbackState, error) {
	cmd := []string{"ectool", "--name=cros_fp", "rollbackinfo"}
	testing.ContextLogf(ctx, "Running command: %s", shutil.EscapeSlice(cmd))
	out, err := d.Conn().CommandContext(ctx, cmd[0], cmd[1:]...).Output(ssh.DumpLogOnError)
	if err != nil {
		return RollbackState{}, errors.Wrap(err, "failed to query FPMCU rollbackinfo")
	}

	var state RollbackState
	err = state.UnmarshalerEctool(out)
	return state, err
}

// AddEntropy adds entropy to the fingerprint MCU.
func AddEntropy(ctx context.Context, d *dut.DUT, reset bool) error {
	args := []string{"addentropy"}
	if reset {
		args = append(args, "reset")
	}
	return EctoolCommand(ctx, d, args[0:]...).Run()
}

// RebootFpmcu reboots the fingerprint MCU. It does not reboot the AP.
func RebootFpmcu(ctx context.Context, d *dut.DUT, bootTo FWImageType) error {
	testing.ContextLog(ctx, "Rebooting FPMCU")
	// This command returns error even on success, so ignore error. b/116396469
	_ = EctoolCommand(ctx, d, "reboot_ec").Run()

	if bootTo == ImageTypeRO {
		// GoBigSleepLint: After verification, FPMCU RO is waiting 1 second for
		// the `rwsigaction abort` command before jumping to RW.
		testing.Sleep(ctx, 500*time.Millisecond)
		err := EctoolCommand(ctx, d, "rwsigaction", "abort").Run()
		if err != nil {
			return errors.Wrap(err, "failed to abort rwsig")
		}
	} else {
		// GoBigSleepLint: Give the FPMCU time to boot before accessing the UART.
		// Otherwise, we could kill the bus and break all comms with the FPMCU.
		testing.Sleep(ctx, 2*time.Second)
	}

	if err := WaitForRunningFirmwareImage(ctx, d, bootTo); err != nil {
		return errors.Wrapf(err, "failed to boot to %q image", bootTo)
	}

	// Double check we are still in the expected image.
	firmwareCopy, err := RunningFirmwareCopy(ctx, d)
	if err != nil {
		return err
	}
	if firmwareCopy != bootTo {
		return errors.Errorf("FPMCU booted to %q, expected %q", firmwareCopy, bootTo)
	}
	return nil
}

// RunningFirmwareCopy returns the firmware copy on FPMCU (RO or RW).
func RunningFirmwareCopy(ctx context.Context, d *dut.DUT) (FWImageType, error) {
	out, err := EctoolCommand(ctx, d, "version").Output()
	if err != nil {
		return FWImageType(""), errors.Wrap(err, "failed to query FPMCU version")
	}
	versionInfoMap := fp.ParseColonDelimitedOutput(string(out))
	firmwareCopy := versionInfoMap["Firmware copy"]
	if firmwareCopy != string(ImageTypeRO) && firmwareCopy != string(ImageTypeRW) {
		return FWImageType(""), errors.New("cannot find firmware copy string")
	}
	return FWImageType(firmwareCopy), nil
}

// CheckFirmwareIsFunctional checks that the AP can talk to the FPMCU and get the version.
func CheckFirmwareIsFunctional(ctx context.Context, d *dut.DUT) ([]byte, error) {
	testing.ContextLog(ctx, "Checking firmware is functional")
	return EctoolCommand(ctx, d, "version").Output(ssh.DumpLogOnError)
}

// WaitForRunningFirmwareImage waits for the requested image to boot.
func WaitForRunningFirmwareImage(ctx context.Context, d *dut.DUT, image FWImageType) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		firmwareCopy, err := RunningFirmwareCopy(ctx, d)
		if err != nil {
			return err
		}
		if firmwareCopy != image {
			return errors.Errorf("FPMCU booted to %q, expected %q", firmwareCopy, image)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 500 * time.Millisecond})
}

// CheckRunningFirmwareCopy validates that image is the running FPMCU firmware copy
// and returns an error if that is not the case.
func CheckRunningFirmwareCopy(ctx context.Context, d *dut.DUT, image FWImageType) error {
	runningImage, err := RunningFirmwareCopy(ctx, d)
	if err != nil {
		return err
	}
	if runningImage != image {
		return errors.Errorf("failed to validate the firmware image, got %q, want %q", runningImage, image)
	}
	return nil
}

// runningFirmwareVersion returns the current RO or RW firmware version on the FPMCU.
func runningFirmwareVersion(ctx context.Context, d *dut.DUT, image FWImageType) (string, error) {
	out, err := EctoolCommand(ctx, d, "version").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to query FPMCU version")
	}
	versionInfoMap := fp.ParseColonDelimitedOutput(string(out))
	switch image {
	case ImageTypeRW:
		return versionInfoMap[ectoolRWVersion], nil
	case ImageTypeRO:
		return versionInfoMap[ectoolROVersion], nil
	default:
		return "", errors.Errorf("unrecognized image type: %q", image)
	}
}

func rawFPFrameCommand(ctx context.Context, d *dut.DUT) *ssh.Cmd {
	return EctoolCommand(ctx, d, "fpframe", "raw")
}

// FpInfoCommand returns the ssh command for running fpinfo.
func FpInfoCommand(ctx context.Context, d *dut.DUT) (*fp.FpInfo, error) {
	out, err := EctoolCommand(ctx, d, "fpinfo").Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, err
	}
	return fp.ParseFpInfo(string(out))
}

// ChipInfoCommand returns the ssh command for running fpinfo.
func ChipInfoCommand(ctx context.Context, d *dut.DUT) (map[string]string, error) {
	out, err := EctoolCommand(ctx, d, "chipinfo").Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, err
	}
	return fp.ParseColonDelimitedOutput(string(out)), nil
}
