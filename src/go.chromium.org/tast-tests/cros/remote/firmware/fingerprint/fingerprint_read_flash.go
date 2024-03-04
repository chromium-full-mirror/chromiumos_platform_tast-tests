// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import (
	"context"

	fp "go.chromium.org/tast-tests/cros/common/fingerprint"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

const (
	rollbackFlashOffsetBloonchipper = "0x20000"
	rollbackFlashOffsetDartmonkey   = "0xe0000"
	rollbackFlashOffsetHelipilot    = "0x20000"
)

func rollbackFlashOffset(fpBoard fp.BoardName) (string, error) {
	switch fpBoard {
	case fp.BoardNameBloonchipper:
		return rollbackFlashOffsetBloonchipper, nil
	case fp.BoardNameDartmonkey, fp.BoardNameNocturne, fp.BoardNameNami:
		return rollbackFlashOffsetDartmonkey, nil
	case fp.BoardNameHelipilot:
		return rollbackFlashOffsetHelipilot, nil
	default:
		return "", errors.Errorf("Rollback flash offset is not defined for %q", fpBoard)
	}
}

// ReadFromRollbackFlashFails attempts to read bytes from the rollback section of the FPMCU's flash.
// Returns error if the rollback section is readable or not able to determine rollback offset.
// The directory containing outputFile must already exist on the DUT.
func ReadFromRollbackFlashFails(ctx context.Context, d *dut.DUT, fpBoard fp.BoardName, outputFile string) error {
	offset, err := rollbackFlashOffset(fpBoard)
	if err != nil {
		return err
	}

	err = EctoolCommand(ctx, d, "flashread", offset, "0x1000", outputFile).Run()
	// Return error if 'ectool' finished successfully.
	if err == nil {
		return errors.Errorf("Flash read from offset %v suceeded", offset)
	}

	return nil
}
