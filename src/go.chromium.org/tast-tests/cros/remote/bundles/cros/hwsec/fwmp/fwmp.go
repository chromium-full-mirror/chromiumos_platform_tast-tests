// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fwmp provides functions and constants for FWMP (Firmware Management Parameters) testing.
package fwmp

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast/core/errors"
)

// Error codes and constants used by fwmp tests.
const (
	// FWMPRemovedErrorCode is the error code returned by GetFirmwareManagementParameters when the FWMP is removed.
	FWMPRemovedErrorCode = "DEVICE_MANAGEMENT_ERROR_FIRMWARE_MANAGEMENT_PARAMETERS_INVALID"

	TestFlags1   = "00000006" // FWMP_DEV_DISABLE_RECOVERY | FWMP_DEV_ENABLE_USB
	TestFlags2   = "0000000c" // FWMP_DEV_ENABLE_USB | FWMP_DEV_ENABLE_LEGACY
	ClearedFlags = "00000000"

	TestHash1   = "0123456789abcdef9876543210abcdef0123456789abcdef9876543210abcdef"
	TestHash2   = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	ClearedHash = "0000000000000000000000000000000000000000000000000000000000000000"
)

// CheckFWMPCleared checks that FWMP is cleared, and returns nil iff it is cleared.
func CheckFWMPCleared(ctx context.Context, utility *hwsec.DeviceManagementClient) error {
	flags, hash, err := utility.GetFirmwareManagementParameters(ctx)

	// There are 2 possible good results, depending on the coreboot and cryptohome implementation.
	// Case 1: If the the FWMP index is owner-defined, invalid space is expected after the clear.
	// Case 2: Otherwise, the FWMP index is platform-defined, which will not be deleted by cryptohome. Instead, cleared flag and hash is expected.
	if err != nil { // Case 1.
		if err.ErrorCode != FWMPRemovedErrorCode {
			return errors.Errorf("call to GetFirmwareManagementParameters failed with an incorrect error code, got %q, want %q", err.ErrorCode, FWMPRemovedErrorCode)
		}
	} else { // Case 2.
		if flags != ClearedFlags || hash != ClearedHash {
			return errors.Errorf("flags or hash not cleared (expecting all 0s); flags: %q, hash: %q", flags, hash)
		}
	}
	return nil
}

// ClearFWMPAndCheck clears FWMP and checks that it's cleared correctly. It return nil iff FWMP is successfully cleared.
func ClearFWMPAndCheck(ctx context.Context, utility *hwsec.DeviceManagementClient) error {
	if _, err := utility.RemoveFirmwareManagementParameters(ctx); err != nil {
		return errors.Wrap(err, "failed to clear fwmp")
	}

	// Note that the reason why we are checking if FWMP is cleared after a
	// successful call to RemoveFirmwareManagementParameters is we want to
	// verify RemoveFirmwareManagementParameters actually does remove FWMP.
	// i.e. We want to catch cases whereby RemoveFirmwareManagementParameters
	// succeeded but it wasn't cleared.
	if err := CheckFWMPCleared(ctx, utility); err != nil {
		return errors.Wrap(err, "failed to check fwmp is cleared")
	}

	return nil
}

// CheckFWMPSet checks that FWMP is set to the expected values.
func CheckFWMPSet(ctx context.Context, utility *hwsec.DeviceManagementClient, expectedFlags, expectedHash string) error {
	flags, hash, err := utility.GetFirmwareManagementParameters(ctx)
	if err != nil {
		return errors.Wrap(err, "call to GetFirmwareManagementParameters failed when trying to check FWMP is set correctly")
	}

	if flags != expectedFlags {
		return errors.Errorf("flags are incorrect when checking FWMP is set correctly, got %q, want %q", flags, expectedFlags)
	}

	if hash != expectedHash {
		return errors.Errorf("hash is incorrect when checking FWMP is set correctly, got %q, want %q", hash, expectedHash)
	}

	return nil
}

// SetFWMPAndCheck sets the FWMP and checks that it's set correctly. It return nil iff FWMP is successfully set.
func SetFWMPAndCheck(ctx context.Context, utility *hwsec.DeviceManagementClient, flags, hash string) error {
	if _, err := utility.SetFirmwareManagementParameters(ctx, flags, hash); err != nil {
		return errors.Wrap(err, "failed to set FWMP")
	}

	// Note that the reason why we are checking if FWMP is set after a
	// successful call to SetFirmwareManagementParameters is we want to
	// verify SetFirmwareManagementParameters actually does set FWMP.
	// i.e. We want to catch cases whereby SetFirmwareManagementParameters
	// succeeded but it wasn't set.
	if err := CheckFWMPSet(ctx, utility, flags, hash); err != nil {
		return errors.Wrap(err, "failed to check fwmp is set correctly")
	}

	return nil
}
