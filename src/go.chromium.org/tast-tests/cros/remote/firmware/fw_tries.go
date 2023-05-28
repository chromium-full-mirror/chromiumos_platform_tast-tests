// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements functions to set/check which firmware the DUT should try to boot from.

package firmware

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

// SetFWTries sets which firmware (A or B) the DUT should boot to next, and how many times it should try booting ino that firmware.
// If tryCount is 0, then fw_try_count will not be modified, only fw_try_next.
func SetFWTries(ctx context.Context, d *dut.DUT, nextFW fwCommon.RWSection, tryCount uint) error {
	if nextFW != fwCommon.RWSectionA && nextFW != fwCommon.RWSectionB {
		return errors.Errorf("unexpected param nextFW: got %s; want A or B", nextFW)
	}
	crossystemMap := make(map[string]string)
	crossystemMap["fw_try_next"] = string(nextFW)
	if tryCount > 0 {
		crossystemMap["fw_try_count"] = strconv.Itoa(int(tryCount))
	}
	// Send crossystem command to set values
	crossystemArgs := make([]string, len(crossystemMap))
	i := 0
	for k, v := range crossystemMap {
		crossystemArgs[i] = fmt.Sprintf("%s=%s", k, v)
		i++
	}
	if err := d.Conn().CommandContext(ctx, "crossystem", crossystemArgs...).Run(); err != nil {
		return errors.Wrapf(err, "running crossystem %s", strings.Join(crossystemArgs, " "))
	}
	return nil
}

// CheckFWTries returns an error if unexpected values are found for the DUT's currently booted firmware, next firmware section to try, or number of tries.
// Optionally, currentFW and nextFW can each take fwCommon.RWSectionUnspecified to avoid checking that section.
func CheckFWTries(ctx context.Context, r *reporters.Reporter, expectedCurrentFW, expectedNextFW fwCommon.RWSection, expectedTryCount uint) error {
	actualCurrentFW, actualNextFW, actualTryCount, err := r.FWTries(ctx)
	if err != nil {
		return errors.Wrap(err, "reporting current FW Tries values")
	}
	currentFWOK := (actualCurrentFW == expectedCurrentFW) || (expectedCurrentFW == fwCommon.RWSectionUnspecified)
	nextFWOK := (actualNextFW == expectedNextFW) || (expectedNextFW == fwCommon.RWSectionUnspecified)
	tryCountOK := actualTryCount == expectedTryCount
	if !currentFWOK || !nextFWOK || !tryCountOK {
		return errors.Errorf("unexpected FW Tries values: got currentFW=%s, nextFW=%s, tryCount=%d; want currentFW=%s, nextFW=%s, tryCount=%d", actualCurrentFW, actualNextFW, actualTryCount, expectedCurrentFW, expectedNextFW, expectedTryCount)
	}
	return nil
}
