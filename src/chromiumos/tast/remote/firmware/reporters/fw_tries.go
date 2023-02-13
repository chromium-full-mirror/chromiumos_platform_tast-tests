// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file reports which firmware the DUT is booted from (A or B),
// which firmware it will try to boot from next, and how many times it will try that firmware.

package reporters

import (
	"context"
	"strconv"

	fwCommon "chromiumos/tast/common/firmware"
	"chromiumos/tast/errors"
)

// FWTries returns the currently booted firmware, the next firmware that should be booted, and the try_count.
func (r *Reporter) FWTries(ctx context.Context) (fwCommon.RWSection, fwCommon.RWSection, uint, error) {
	csMap, err := r.Crossystem(ctx, CrossystemParamMainfwAct, CrossystemParamFWTryNext, CrossystemParamFWTryCount)
	if err != nil {
		return fwCommon.RWSectionUnspecified, fwCommon.RWSectionUnspecified, 0, err
	}
	currentFW := fwCommon.RWSection(csMap[CrossystemParamMainfwAct])
	nextFW := fwCommon.RWSection(csMap[CrossystemParamFWTryNext])
	tryCount, err := strconv.ParseUint(csMap[CrossystemParamFWTryCount], 10, 32)
	if err != nil {
		return fwCommon.RWSectionUnspecified, fwCommon.RWSectionUnspecified, 0, errors.Wrapf(err, "unexpected crossystem value for %s: got %s; want uint", CrossystemParamFWTryCount, csMap[CrossystemParamFWTryCount])
	}
	return currentFW, nextFW, uint(tryCount), nil
}
