// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/errors"
)

// AddPerformanceCUJMetrics adds the metrics to the recorder for performance CUJ test.
func AddPerformanceCUJMetrics(bt browser.Type, tconn, bTconn *chrome.TestConn, recorder *cujrecorder.Recorder) error {
	ashMetrics := cujrecorder.AshCommonMetricConfigs()
	lacrosMetrics := cujrecorder.CUJLacrosCommonMetricConfigs()
	browserMetrics := cujrecorder.BrowserCommonMetricConfigs()
	commonMetrics := cujrecorder.AnyChromeCommonMetricConfigs()

	// Collect all metrics from all browsers to make it compatible with the CUJ scores generated
	// from previouse releases, which collects all metrics for all system activities.
	allMetrics := append(commonMetrics, append(ashMetrics, browserMetrics...)...)
	if err := recorder.AddCollectedMetrics(tconn, browser.TypeAsh, allMetrics...); err != nil {
		errors.Wrap(err, "failed to add metrics for tconn")
	}
	if bt == browser.TypeLacros {
		allMetrics = append(allMetrics, lacrosMetrics...)
		if err := recorder.AddCollectedMetrics(bTconn, browser.TypeLacros, allMetrics...); err != nil {
			errors.Wrap(err, "failed to add metrics for bTconn")
		}
	}
	return nil
}
