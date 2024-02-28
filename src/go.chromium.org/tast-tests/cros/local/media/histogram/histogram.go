// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package histogram is a package for common code related to video specific histogram.
package histogram

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SuccessCountAtLeastOne is a constant for the sentinel value of -1 that can
// be used to let WasHWAccelUsed know that we want a success count >= 1
// instead of equal to a concrete value.
const SuccessCountAtLeastOne = -1

// WasHWAccelUsed returns whether HW acceleration is used for certain action.
// initHistogram is the histogram obtained before the action.
// successValue is the bucket value of HW acceleration success case.
// successCount is the expected number of successful counts.
// It has been observed that in some cases, we occasionally see more than one
// successful count (crbug.com/985068) when only one count is expected.
// To prevent it from reporting false negative results, we can relax the condition
// by checking that the count is >= 1 instead of equal to 1, regardless it may
// introduce some false positive results. To indicate we want to relax the check
// this way we just need to pass a successCount of -1 (the constant
// SuccessCountAtLeastOne defined above can be used for this).
func WasHWAccelUsed(ctx context.Context, tconn *chrome.TestConn, initHistogram *histogram.Histogram, histogramName string, successValue, successCount int64) (bool, error) {
	// There are three valid cases.
	// 1. No histogram is updated. This is the case if HW Acceleration is disabled due to Chrome flag, ex. --disable-accelerated-video-decode.
	// 2. Histogram is updated with 15. This is the case if Chrome tries to initailize HW Acceleration but it fails because the codec is not supported on DUT.
	// 3. Histogram is updated with 0. This is the case if Chrome sucessfully initializes HW Acceleration.

	// err is not nil here if HW Acceleration is disabled and then Chrome doesn't try HW Acceleration initialization at all.
	// For the case 1, we pass a short time context to WaitForHistogramUpdate to avoid the whole test context (ctx) from reaching deadline.
	histogramDiff, err := metrics.WaitForHistogramUpdate(ctx, tconn, histogramName, initHistogram, 15*time.Second)
	if err != nil {
		// This is the first case; no histogram is updated.
		return false, nil
	}

	testing.ContextLogf(ctx, "Got update to %s histogram: %s", histogramName, histogramDiff)
	if len(histogramDiff.Buckets) > 1 {
		return false, errors.Wrapf(err, "unexpected histogram update: %v", histogramDiff)
	}

	diff := histogramDiff.Buckets[0]

	var hwAccelUsed bool
	if successCount == SuccessCountAtLeastOne {
		hwAccelUsed = diff.Min == successValue && diff.Max == successValue+1 && diff.Count >= 1
	} else {
		hwAccelUsed = diff.Min == successValue && diff.Max == successValue+1 && diff.Count == successCount
	}

	if !hwAccelUsed {
		var targetCount int64
		if successCount == SuccessCountAtLeastOne {
			targetCount = 1
		} else {
			targetCount = successCount
		}

		testing.ContextLogf(ctx, "Histogram update: %s, if HW accel were used, it should be [[%d, %d) %d]", histogramDiff, successValue, successValue+1, targetCount)
	} else if diff.Count > 1 && successCount == SuccessCountAtLeastOne {
		testing.ContextLog(ctx, "Note that more than one successful count is observed: ", diff.Count)
	}

	return hwAccelUsed, nil
}
