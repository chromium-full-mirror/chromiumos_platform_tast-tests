// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/metrics"
	"chromiumos/tast/testing"
)

// RunAndWaitLCPHistograms runs the function and waits for the LCP2 and TTFB histograms to update.
func RunAndWaitLCPHistograms(ctx context.Context, bTconn *chrome.TestConn, f func(ctx context.Context) error) error {
	histograms, err := metrics.RunAndWaitAll(ctx, bTconn, time.Second, f,
		"PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
		"PageLoad.Experimental.NavigationTiming.NavigationStartToFirstResponseStart",
	)
	if err != nil {
		return errors.Wrap(err, "failed to collect histograms")
	}
	testing.ContextLog(ctx, "Collect histograms: ", histograms)
	return nil
}

// WaitForPDFVideoHistogram waits for "Graphics.Smoothness.PercentDroppedFrames3.CompositorThread.Video" histogram to be available.
func WaitForPDFVideoHistogram(ctx context.Context, bTconn *chrome.TestConn) error {
	const videoHistogramName = "Graphics.Smoothness.PercentDroppedFrames3.CompositorThread.Video"
	startTime := time.Now()
	histogram, err := metrics.WaitForHistogram(ctx, bTconn, videoHistogramName, 5*time.Second)
	if err != nil {
		return err
	}
	testing.ContextLogf(ctx, "Collected %v in %v", histogram, time.Since(startTime))
	return nil
}
