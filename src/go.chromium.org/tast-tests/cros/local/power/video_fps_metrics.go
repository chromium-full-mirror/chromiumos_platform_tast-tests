// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/testing"
)

// VideoFpsMetrics records the fps of video elements in a page.
type VideoFpsMetrics struct {
	conn         *chrome.Conn
	metrics      map[string]perf.Metric
	lastTime     time.Time
	lastValues   []int
	names        []string
	prefix       string
	intervalName string
}

// Assert that VideoFpsMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &VideoFpsMetrics{}

// NewVideoFpsMetrics creates the struct to store video FPS metrics.
func NewVideoFpsMetrics(conn *chrome.Conn) *VideoFpsMetrics {
	newMetrics := &VideoFpsMetrics{
		conn:    conn,
		metrics: make(map[string]perf.Metric),
	}
	return newMetrics
}

// Setup collects the prefix and interval name.
// We want to do the actual metrics setup in Start() because we need to wait for test
// to setup chrome tab and play the video there.
func (v *VideoFpsMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	v.prefix = prefix
	v.intervalName = intervalName
	return nil
}

// Start logs the start of video fps stats tracker. Read first snapshot of the stats.
// This function is required by perf.Timeline.
func (v *VideoFpsMetrics) Start(ctx context.Context) error {
	const js = "Array.from(document.getElementsByTagName('video')).map(v => v.id)"
	var videoNames []string
	if err := v.conn.Eval(ctx, js, &videoNames); err != nil {
		return err
	}
	// Generate video name if it does not have one.
	for i := 0; i < len(videoNames); i++ {
		if videoNames[i] == "" {
			videoNames[i] = "video_" + strconv.Itoa(i)
		}
	}
	v.names = videoNames
	for _, videoName := range videoNames {
		v.metrics[videoName] = perf.Metric{
			Name:      v.prefix + cp.FPSMetricType + videoName,
			Unit:      cp.FPSMetricTypeUnit,
			Direction: perf.BiggerIsBetter,
			Multiple:  true,
			Interval:  v.intervalName,
		}
	}

	testing.ContextLog(ctx, "Start tracking video fps stats")
	values, t, err := readVideoFrameCount(ctx, v.conn)
	if err != nil {
		return err
	}
	v.lastValues = values
	v.lastTime = t
	return nil
}

// Snapshot takes one snapshot of video fps stats.
func (v *VideoFpsMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	newValues, newTime, err := readVideoFrameCount(ctx, v.conn)
	if err != nil {
		return err
	}
	duration := newTime.Sub(v.lastTime).Seconds()
	for i, n := range v.names {
		fps := float64(newValues[i]-v.lastValues[i]) / duration
		values.Append(v.metrics[n], fps)
	}
	v.lastValues = newValues
	v.lastTime = newTime
	return nil
}

// Stop logs the stop of video fps stats tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (v *VideoFpsMetrics) Stop(ctx context.Context, values *perf.Values) error {
	testing.ContextLog(ctx, "Stop tracking video fps stats")
	return nil
}

// readVideoFrameCount returns webkitDecodedFrameCount for all video in a page and current time.
func readVideoFrameCount(ctx context.Context, conn *chrome.Conn) ([]int, time.Time, error) {
	const js = "Array.from(document.getElementsByTagName('video')).map(v => v.webkitDecodedFrameCount)"
	var framesCount []int
	if err := conn.Eval(ctx, js, &framesCount); err != nil {
		return nil, time.Now(), err
	}
	return framesCount, time.Now(), nil
}
