// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/errors"
)

// AddPerformanceCUJMetrics adds the metrics to the recorder for performance CUJ test.
func AddPerformanceCUJMetrics(recorder *cujrecorder.Recorder) error {
	ashMetrics := cujrecorder.AshCommonMetricConfigs()
	browserMetrics := cujrecorder.BrowserCommonMetricConfigs()
	commonMetrics := cujrecorder.AnyChromeCommonMetricConfigs()

	// Collect all metrics to make it compatible with the CUJ scores generated from
	// previouse releases, which collects all metrics for all system activities.
	allMetrics := append(commonMetrics, append(ashMetrics, browserMetrics...)...)
	if err := recorder.AddCollectedMetrics(allMetrics...); err != nil {
		errors.Wrap(err, "failed to add metrics")
	}
	return nil
}
