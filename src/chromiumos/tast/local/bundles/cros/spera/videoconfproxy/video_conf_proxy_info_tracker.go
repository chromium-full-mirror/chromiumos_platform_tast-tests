// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconfproxy

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// InfoTracker is a helper to collect video conf proxy info.
type InfoTracker struct {
	tconn         *chrome.TestConn
	videoConn     *chrome.Conn
	srcFrameRate  []float64
	outFrameRate  []float64
	collecting    chan bool
	collectingErr chan error
	err           error
}

// NewVideoConfProxyInfoTracker creates a new instance of InfoTracker.
func NewVideoConfProxyInfoTracker(ctx context.Context, tconn *chrome.TestConn, videoConn *chrome.Conn) *InfoTracker {
	return &InfoTracker{tconn: tconn, videoConn: videoConn}
}

func (t *InfoTracker) getFrameRates(ctx context.Context) (err error) {
	// Wait until the src text appears to ensure the data can be collected.
	ui := uiauto.New(t.tconn)
	srcFinder := nodewith.NameContaining("src: ").Role(role.StaticText)
	if err := ui.WaitUntilExists(srcFinder)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for src text to appear")
	}

	var frameRates []float64
	if err := t.videoConn.Call(ctx, &frameRates, `() => {
		let stats = window.outboundWebRTCStats;
		return [stats.srcFramesPerSecond, stats.framesPerSecond]
		}
		`); err != nil {
		return errors.Wrap(err, "failed to get frame rates")
	}
	if len(frameRates) != 2 {
		return errors.Errorf("got wrong number of frame rate data, got %d, want %d", len(frameRates), 2)
	}
	srcFrameRate := frameRates[0]
	outFrameRate := frameRates[1]

	t.srcFrameRate = append(t.srcFrameRate, float64(srcFrameRate))
	t.outFrameRate = append(t.outFrameRate, float64(outFrameRate))

	return nil
}

// Start indicates that the video conf proxy tracking should start.
func (t *InfoTracker) Start(ctx context.Context) error {
	if t == nil {
		return errors.New("video conf proxy info tracker is not provided to start")
	}
	t.collecting = make(chan bool)
	t.collectingErr = make(chan error, 1)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-t.collecting:
				close(t.collectingErr)
				return
			case <-ticker.C:
				if err := t.getFrameRates(ctx); err != nil {
					t.collectingErr <- err
					return
				}
			case <-ctx.Done():
				t.collectingErr <- ctx.Err()
				return
			}
		}
	}()
	return nil
}

// Stop indicates that the video conf proxy tracking should stop.
func (t *InfoTracker) Stop(ctx context.Context) error {
	if t == nil {
		return errors.New("video conf proxy info tracker is not provided to stop")
	}
	// Stop collecting go routine.
	close(t.collecting)
	select {
	case err := <-t.collectingErr:
		if err != nil {
			testing.ContextLog(ctx, "Failed to collect VideoConfProxy info: ", err)
			t.err = err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// Record stores the collected data into pv for further processing.
func (t *InfoTracker) Record(ctx context.Context, pv *perf.Values) error {
	if t == nil {
		return errors.New("video conf proxy info tracker is not provided to record")
	}
	if t.err != nil {
		return t.err
	}

	var avgSrcFrameRate, sumSrcFrameRate, avgOutFrameRate, sumOutFrameRate float64
	for _, frameRate := range t.srcFrameRate {
		sumSrcFrameRate += frameRate
	}
	avgSrcFrameRate = sumSrcFrameRate / float64(len(t.srcFrameRate))

	for _, frameRate := range t.outFrameRate {
		sumOutFrameRate += frameRate
	}
	avgOutFrameRate = sumOutFrameRate / float64(len(t.outFrameRate))

	var frames []float64
	if err := t.videoConn.Call(ctx, &frames, `() => {
		let stats = window.outboundWebRTCStats;
		return [stats.srcFrames, stats.framesSent]
		}
		`); err != nil {
		return errors.Wrap(err, "failed to get frame data")
	}
	if len(frames) != 2 {
		return errors.Errorf("got wrong number of frame data, got %d, want %d", len(frames), 2)
	}
	srcFrames := frames[0]
	outFrames := frames[1]
	if outFrames > srcFrames {
		return errors.Errorf("out frames %f should not be larger than src frames %f", outFrames, srcFrames)
	}
	testing.ContextLogf(ctx, "Camera src frame count: %f, out frame count: %f", srcFrames, outFrames)
	droppedFrames := srcFrames - outFrames
	droppedFrameRate := droppedFrames / srcFrames

	pv.Set(perf.Metric{
		Name:      "VideoConfProxy.Src.FrameRate",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}, t.srcFrameRate...)
	pv.Set(perf.Metric{
		Name:      "VideoConfProxy.Src.Average.FrameRate",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
	}, avgSrcFrameRate)
	pv.Set(perf.Metric{
		Name:      "VideoConfProxy.Out.FrameRate",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}, t.outFrameRate...)
	pv.Set(perf.Metric{
		Name:      "VideoConfProxy.Out.Average.FrameRate",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
	}, avgOutFrameRate)
	pv.Set(perf.Metric{
		Name:      "VideoConfProxy.DroppedFrames",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, droppedFrames)
	pv.Set(perf.Metric{
		Name:      "VideoConfProxy.DroppedFrameRate",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, droppedFrameRate)
	return nil
}
