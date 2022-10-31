// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconfproxy

import (
	"context"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

// InfoTracker is a helper to collect video conf proxy info.
type InfoTracker struct {
	tconn         *chrome.TestConn
	srcFrameRate  []float64
	outFrameRate  []float64
	collecting    chan bool
	collectingErr chan error
	err           error
}

// NewVideoConfProxyInfoTracker creates a new instance of InfoTracker.
func NewVideoConfProxyInfoTracker(ctx context.Context, tconn *chrome.TestConn) *InfoTracker {
	return &InfoTracker{tconn: tconn}
}

func (t *InfoTracker) getFrameRates(ctx context.Context) error {
	ui := uiauto.New(t.tconn)
	srcFinder := nodewith.NameContaining("src: ").Role(role.StaticText)
	outFinder := nodewith.NameContaining("out: ").Role(role.StaticText)

	srcInfo, err := ui.NodesInfo(ctx, srcFinder)
	if err != nil {
		return err
	}
	if len(srcInfo) == 0 {
		return errors.New("there is no src frame rate info")
	}
	srcName := srcInfo[0].Name
	srcFrameRateStr := strings.Split(strings.Split(srcName, "@")[1], ";")[0]
	srcFrameRateFloat, err := strconv.ParseFloat(srcFrameRateStr, 64)
	if err != nil {
		return err
	}
	t.srcFrameRate = append(t.srcFrameRate, srcFrameRateFloat)

	outInfo, err := ui.NodesInfo(ctx, outFinder)
	if err != nil {
		return err
	}
	if len(outInfo) == 0 {
		return errors.New("there is no out frame rate info")
	}
	outName := outInfo[0].Name
	outFrameRateStr := strings.Split(strings.Split(outName, "@")[1], " ")[0]
	outFrameRateFloat, err := strconv.ParseFloat(outFrameRateStr, 64)
	if err != nil {
		return err
	}
	t.outFrameRate = append(t.outFrameRate, outFrameRateFloat)
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
func (t *InfoTracker) Record(pv *perf.Values) error {
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
	return nil
}
