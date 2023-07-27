// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package updateutil

import (
	"context"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

const (
	// The crossystem `fwTryCountFlag` is set to `resetFWTryCount`
	// when a firmware update is invalidated, to indicate
	// that currently booted firmware successfully booted.
	resetFWTryCount = "0"
	// Arbitrary value to set the crossystem `fwTryCountFlag` to
	// to indicate the number of tries to try the next firmware update.
	// Used to fake a firmware update.
	updateFWTryCount = "5"
)

// FakeFirmwareUpdate fakes a firmware update by setting appropriate crossystem
// flags indicating the update.
// Sets the next FW slot to try on next boot to an alternative slot.
func FakeFirmwareUpdate(ctx context.Context, dut *dut.DUT) error {
	// Get the currently booted FW partition slot.
	mainFWAct, err := GetCrossystemFlag(ctx, dut, MainFWActFlag)
	if err != nil {
		return errors.Wrap(err, "failed to get current firmware partition")
	}

	// Decide the alternative FW slot.
	var fwNext string
	if mainFWAct == "A" {
		fwNext = "B"
	} else if mainFWAct == "B" {
		fwNext = "A"
	} else {
		return errors.New("failed to find alternative FW slot")
	}

	// Set the fw_try_next crossystem flag to change the next FW slot to try.
	if err := SetCrossystemFlag(ctx, dut, FWTryNextFlag, fwNext); err != nil {
		return errors.Wrap(err, "failed to change next FW slot")
	}

	// Set the fw_try_count crossystem flag to indicate number of tries
	// to try the next FW slot.
	if err := SetCrossystemFlag(ctx, dut, FWTryCountFlag, updateFWTryCount); err != nil {
		return errors.Wrap(err, "failed to change FW slot try count")
	}

	return nil
}

// RevertFakeFirmwareUpdate reverts crossystem flags set by FakeFirmwareUpdate.
func RevertFakeFirmwareUpdate(ctx context.Context, dut *dut.DUT) error {
	// Get the currently booted FW partition slot.
	mainFWAct, err := GetCrossystemFlag(ctx, dut, MainFWActFlag)
	if err != nil {
		return errors.Wrap(err, "failed to get current firmware partition")
	}

	// Set the fw_try_next crossystem flag to the currently booted FW partition.
	if err := SetCrossystemFlag(ctx, dut, FWTryNextFlag, mainFWAct); err != nil {
		return errors.Wrap(err, "failed to change next FW slot")
	}

	// Set the fw_try_count crossystem flag to zero.
	if err := SetCrossystemFlag(ctx, dut, FWTryCountFlag, resetFWTryCount); err != nil {
		return errors.Wrap(err, "failed to change FW slot try count")
	}

	return nil
}

// VerifyInvalidatedFirmwareUpdate verifies that a firmware update
// is reset properly.
func VerifyInvalidatedFirmwareUpdate(ctx context.Context, dut *dut.DUT, preUpdateFwAct string) error {
	// Get the currently booted FW partition slot.
	mainFWAct, err := GetCrossystemFlag(ctx, dut, MainFWActFlag)
	if err != nil {
		return errors.Wrap(err, "failed to read main_fw_act from crossystem")
	}

	// Get a value of the fw_try_next flag.
	fwTryNext, err := GetCrossystemFlag(ctx, dut, FWTryNextFlag)
	if err != nil {
		return errors.Wrap(err, "failed to read fw_try_next from crossystem")
	}

	// Get a value of the fw_try_count flag.
	fwTryCount, err := GetCrossystemFlag(ctx, dut, FWTryCountFlag)
	if err != nil {
		return errors.Wrap(err, "failed to read fw_try_count from crossystem")
	}

	// Get a value of the fw_result flag.
	fwResult, err := GetCrossystemFlag(ctx, dut, FWResultFlag)
	if err != nil {
		return errors.Wrap(err, "failed to read fw_result from crossystem")
	}

	// Verify that flags are properly reset.
	if mainFWAct != preUpdateFwAct || mainFWAct != fwTryNext || fwTryCount != resetFWTryCount || fwResult != "success" {
		return errors.New("Firmware update is not invalidated")
	}

	return nil
}
