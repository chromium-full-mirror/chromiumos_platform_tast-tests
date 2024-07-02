// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/async"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	frameDataFetchInterval = time.Minute
	throughputInterval     = 5 * time.Second
	frameSinkBucketSize    = 1
)

// FrameDataTracker is helper to get animation frame data from Chrome.
type FrameDataTracker struct {
	prefix         string
	animationData  []DisplayFrameData
	dsData         *DisplayFrameData
	frameCountData []FrameCountingPerSinkData
	dsTracker      *DisplaySmoothnessTracker
	timeOffset     time.Duration
	collecting     chan bool
	collectingErr  chan error
}

// FrameCountingPerSinkData holds collected frame counts for a sink type.
type FrameCountingPerSinkData struct {
	SinkType        string `json:"sinkType"`
	IsRoot          bool   `json:"isRoot"`
	DebugLabel      string `json:"debugLabel"`
	PresentedFrames []int  `json:"presentedFrames"`
}

// Close ensures that the browser state (display smoothness tracking) is cleared.
func (t *FrameDataTracker) Close(ctx context.Context, tconn *chrome.TestConn) error {
	return t.dsTracker.Close(ctx, tconn)
}

// Start starts the animation data collection.
func (t *FrameDataTracker) Start(ctx context.Context, tconn *chrome.TestConn, timeZero time.Time) error {
	if t.collecting != nil {
		return errors.New("already started")
	}

	// Use a short 5s delay for stop.
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	t.collecting = make(chan bool)
	if err := t.forceStop(stopCtx, tconn); err != nil {
		testing.ContextLog(ctx, "Failed to stop any existing frame data tracking: ", err)
	}

	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.startThroughputTrackerDataCollection)`); err != nil {
		return errors.Wrap(err, "failed to start data collection")
	}

	// Start frame counting with a bucket size of 1, to capture frames
	// per second. Assume the start time of the timeline is right before
	// autotestPrivate.startFrameCounting is called.
	t.timeOffset = time.Since(timeZero)
	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.startFrameCounting)`, frameSinkBucketSize); err != nil {
		return errors.Wrap(err, "failed to start frame counting per sink")
	}

	if err := t.dsTracker.Start(ctx, tconn, "", throughputInterval); err != nil {
		return errors.Wrap(err, "failed to start display smoothness tracking")
	}

	t.collectingErr = make(chan error, 1)

	async.Run(ctx, func(ctx context.Context) {
		testing.ContextLog(ctx, "FrameDataTracker: Collecting frame data in background")
		for {
			select {
			case <-t.collecting:
				close(t.collectingErr)
				return
			case <-time.After(frameDataFetchInterval):
				var data []DisplayFrameData
				if err := tconn.Call(ctx, &data, `tast.promisify(chrome.autotestPrivate.getThroughputTrackerData)`); err != nil {
					t.collectingErr <- errors.Wrap(err, "failed to get collected data")
					return
				}
				t.animationData = append(t.animationData, data...)
			case <-ctx.Done():
				t.collectingErr <- ctx.Err()
				return
			}
		}
	}, "FrameDataTracker")

	return nil
}

// forceStop clears any existing frame data tracking, and ignores any potential
// output from the trackers.
func (t *FrameDataTracker) forceStop(ctx context.Context, tconn *chrome.TestConn) error {
	if t.collecting == nil {
		return errors.New("not started")
	}

	if _, err := t.dsTracker.Stop(ctx, tconn, ""); err != nil {
		testing.ContextLog(ctx, errors.Wrap(err, "failed to stop display smoothness tracking"))
	}

	var data []DisplayFrameData
	if err := tconn.Call(ctx, &data, `tast.promisify(chrome.autotestPrivate.stopThroughputTrackerDataCollection)`); err != nil {
		testing.ContextLog(ctx, errors.Wrap(err, "failed to stop data collection"))
	}

	var frameCountData []FrameCountingPerSinkData
	if err := tconn.Call(ctx, &frameCountData, `tast.promisify(chrome.autotestPrivate.stopFrameCounting)`); err != nil {
		testing.ContextLog(ctx, errors.Wrap(err, "failed to stop frame counting per sink"))
	}

	return nil
}

// Stop stops the animation data collection and stores the collected data.
func (t *FrameDataTracker) Stop(ctx context.Context, tconn *chrome.TestConn) error {
	if t.collecting == nil {
		return errors.New("not started")
	}
	close(t.collecting)

	var firstErr error
	select {
	case firstErr = <-t.collectingErr:
	case <-ctx.Done():
		return ctx.Err()
	}

	var dsData *DisplayFrameData
	var err error
	if dsData, err = t.dsTracker.Stop(ctx, tconn, ""); err != nil {
		if firstErr == nil {
			firstErr = errors.Wrap(err, "failed to stop display smoothness tracking")
		}
	}

	var data []DisplayFrameData
	if err := tconn.Call(ctx, &data, `tast.promisify(chrome.autotestPrivate.stopThroughputTrackerDataCollection)`); err != nil {
		if firstErr == nil {
			firstErr = errors.Wrap(err, "failed to stop data collection")
		}
	}

	var frameCountData []FrameCountingPerSinkData
	if err := tconn.Call(ctx, &frameCountData, `tast.promisify(chrome.autotestPrivate.stopFrameCounting)`); err != nil {
		if firstErr == nil {
			firstErr = errors.Wrap(err, "failed to stop frame counting per sink")
		}
	}

	if firstErr != nil {
		return firstErr
	}

	t.dsData = dsData
	t.animationData = append(t.animationData, data...)
	t.frameCountData = frameCountData
	return nil
}

// Record stores the collected data into pv for further processing.
func (t *FrameDataTracker) Record(pv *perf.Values) {
	startTimeMetric := perf.Metric{
		Name:     t.prefix + "Animation.StartTimeOffsetMs",
		Unit:     "ms",
		Multiple: true,
	}
	stopTimeMetric := perf.Metric{
		Name:     t.prefix + "Animation.StopTimeOffsetMs",
		Unit:     "ms",
		Multiple: true,
	}
	feMetric := perf.Metric{
		Name:      t.prefix + "Animation.FramesExpected",
		Unit:      "count",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}
	fpMetric := perf.Metric{
		Name:      t.prefix + "Animation.FramesProduced",
		Unit:      "count",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}
	jcMetric := perf.Metric{
		Name:      t.prefix + "Animation.JankCount",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}

	for _, data := range t.animationData {
		pv.Append(startTimeMetric, float64(data.StartOffsetMs))
		pv.Append(stopTimeMetric, float64(data.StopOffsetMs))
		pv.Append(feMetric, float64(data.FramesExpected))
		pv.Append(fpMetric, float64(data.FramesProduced))
		pv.Append(jcMetric, float64(data.JankCount))
	}

	// Keep track of how many of each sink types have been seen to
	// properly label the metric name.
	sinkTypeCounts := make(map[string]int)
	var numBuckets int
	r := regexp.MustCompile(`[a-zA-Z0-9]{1,50}`)
	for _, sink := range t.frameCountData {
		// Debug label might contain illegal characters (e.g. ",").
		// Join the string by separator "-" because "-" is allowed
		// in metric name.
		matches := r.FindAllString(sink.DebugLabel, -1)
		debugLabel := strings.Join(matches, "-")
		var sinkName string
		if debugLabel != "" {
			sinkName = "." + sink.SinkType + "." + debugLabel[0:int(math.Min(50, float64(len(debugLabel))))]
		} else {
			sinkName = "." + sink.SinkType
		}
		if sink.IsRoot {
			sinkName = ".root" + sinkName
		}

		// Metrics are named according to the following:
		// "<prefix>FrameSink<optional .root>.<sink type>.<debug label>.<current number of that sink name>"
		//
		// i.e:
		// TPS.FrameSink.root.layer-tree.0
		// TPS.FrameSink.unspecified.0
		frameSinkMetric := perf.Metric{
			Name:      fmt.Sprintf("%sFrameSink%s.%d", t.prefix, sinkName, sinkTypeCounts[sinkName]),
			Unit:      "count",
			Direction: perf.BiggerIsBetter,
			Multiple:  true,
			Interval:  fmt.Sprintf("%vs", frameSinkBucketSize),
		}

		for _, count := range sink.PresentedFrames {
			pv.Append(frameSinkMetric, float64(count))
		}
		sinkTypeCounts[sinkName]++

		if sinkBuckets := len(sink.PresentedFrames); sinkBuckets > numBuckets {
			numBuckets = sinkBuckets
		}
	}

	frameSinkTime := perf.Metric{
		Name:     "FrameSink.t",
		Unit:     "s",
		Multiple: true,
	}
	secondsOffset := t.timeOffset.Seconds()
	for i := 0; i < numBuckets; i++ {
		pv.Append(frameSinkTime, float64(i*frameSinkBucketSize)+secondsOffset)
	}

	// FrameData collecting on the DUTs may fail (b/210185705) or return no data.
	// Check if data is collected before recording it.
	if t.dsData == nil {
		return
	}

	pv.Set(perf.Metric{
		Name:      t.prefix + "DisplayJankMetric",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, float64(t.dsData.JankCount)/float64(t.dsData.FramesExpected)*100)

	pv.Set(perf.Metric{
		Name:      t.prefix + "Display.FramesExpected",
		Unit:      "count",
		Direction: perf.BiggerIsBetter,
	}, float64(t.dsData.FramesExpected))
	pv.Set(perf.Metric{
		Name:      t.prefix + "Display.FramesProduced",
		Unit:      "count",
		Direction: perf.BiggerIsBetter,
	}, float64(t.dsData.FramesProduced))
	pv.Set(perf.Metric{
		Name:      t.prefix + "Display.JankCount",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, float64(t.dsData.JankCount))

	smMetric := perf.Metric{
		Name:      t.prefix + "Display.Smoothness",
		Multiple:  true,
		Unit:      "percent",
		Direction: perf.BiggerIsBetter,
		Interval:  fmt.Sprintf("%vs", throughputInterval.Seconds()),
	}
	for _, data := range t.dsData.Throughput {
		pv.Append(smMetric, float64(data))
	}

	jdMetric := perf.Metric{
		Name:      t.prefix + "Display.JankDurations",
		Multiple:  true,
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}
	for _, data := range t.dsData.JankDurations {
		pv.Append(jdMetric, data)
	}
}

// NewFrameDataTracker creates a new instance for FrameDataTracker.
func NewFrameDataTracker(metricPrefix string) (*FrameDataTracker, error) {
	return &FrameDataTracker{
		prefix:    metricPrefix,
		dsTracker: NewDisplaySmoothnessTracker(),
	}, nil
}
