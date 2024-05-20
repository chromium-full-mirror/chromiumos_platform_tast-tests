// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/bounds"
	"go.chromium.org/tast/core/testing"
)

// FatalIfBoundsCheckFail checks whether the test results fell within
// the provided metric bounds.
func FatalIfBoundsCheckFail(ctx context.Context, metricBounds []bounds.MetricBounds, s *testing.State) {
	if err := bounds.EvaluateResults(ctx, metricBounds, s.OutDir()); err != nil {
		s.Fatal("Failed bounds check: ", err)
	}
}
