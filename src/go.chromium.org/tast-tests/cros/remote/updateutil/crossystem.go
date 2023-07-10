// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package updateutil

import (
	"context"
	"fmt"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

// Firmware crossystem flags.
// References same flags used in firmware updater
// in chromiumos/src/platform/vboot_reference/updater.c:set_try_cookies.
const (
	// The next FW slot to try on next boot.
	FWTryNextFlag = "fw_try_next"
	// The currently booted FW slot.
	MainFWActFlag = "mainfw_act"
	// How many times to try the next FW slot.
	FWTryCountFlag = "fw_try_count"
	// Boot result of the currently booted FW slot.
	FWResultFlag = "fw_result"
)

// SetCrossystemFlag sets a crossystem `flag` to `value`.
func SetCrossystemFlag(ctx context.Context, dut *dut.DUT, flag, value string) error {
	if err := dut.Conn().CommandContext(ctx, "crossystem", fmt.Sprintf("%s=%s", flag, value)).Run(); err != nil {
		return errors.Wrapf(err, "failed to set crossystem %s=%s", flag, value)
	}

	return nil
}

// GetCrossystemFlag returns a value of a crossystem `flag`.
func GetCrossystemFlag(ctx context.Context, dut *dut.DUT, flag string) (string, error) {
	value, err := dut.Conn().CommandContext(ctx, "crossystem", flag).Output()
	if err != nil {
		return "", errors.Wrapf(err, "failed to read crossystem %s", flag)
	}

	return string(value), nil
}
