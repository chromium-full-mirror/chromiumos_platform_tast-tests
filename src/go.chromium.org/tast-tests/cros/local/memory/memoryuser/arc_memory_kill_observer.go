// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ArcMemoryKillInfo describes a kill event that occurred in Android.
type ArcMemoryKillInfo struct {
	Time    time.Time
	Package string
}

// ArcMemoryKillObserver monitors logcat to record LMKD and
// ApplyHostMemoryPressure kill events.
type ArcMemoryKillObserver struct {
	cancel context.CancelFunc

	// The first kill event of the given priority that occurred after this
	// observer was constructed or reset.
	Cached      *ArcMemoryKillInfo
	Perceptible *ArcMemoryKillInfo
	Foreground  *ArcMemoryKillInfo

	// The most recent error encountered by this observer.
	Error error
}

// Close causes this observer to stop monitoring for new kill events.
func (o *ArcMemoryKillObserver) Close() {
	o.cancel()
}

// Reset clears any recorded kill events to allow new ones to be recorded.
func (o *ArcMemoryKillObserver) Reset() {
	o.Cached = nil
	o.Perceptible = nil
	o.Foreground = nil
}

var pressureKillRE = regexp.MustCompile(`^([0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}) +[0-9]+ +[0-9]+ I ArcProcessService: ApplyHostMemoryPressure\((CACHED|PERCEPTIBLE|FOREGROUND)\) killed ([^ ]+) `)
var lmkdKillRE = regexp.MustCompile(`^([0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}) +[0-9]+ +[0-9]+ I lowmemorykiller: Kill '([^']+)' \([0-9]+\), uid [0-9]+, oom_(?:score_)?adj ([0-9]+) `)

func (o *ArcMemoryKillObserver) observeLine(ctx context.Context, line string) {
	var recordLocation **ArcMemoryKillInfo
	var timeString, packageString string
	if groups := pressureKillRE.FindStringSubmatch(line); groups != nil {
		timeString = groups[1]
		packageString = groups[3]

		priString := groups[2]
		switch priString {
		case "CACHED":
			recordLocation = &o.Cached
		case "PERCEPTIBLE":
			recordLocation = &o.Perceptible
		case "FOREGROUND":
			recordLocation = &o.Foreground
		default:
			testing.ContextLogf(ctx, "Warning: ArcMemoryKillObserver unknown pressure kill priority %q", priString)
			return
		}
		testing.ContextLogf(ctx, "ApplyHostMemoryPressure kill observed, %q, %s", packageString, priString)
	} else if groups := lmkdKillRE.FindStringSubmatch(line); groups != nil {
		timeString = groups[1]
		packageString = groups[2]

		oomAdjString := groups[3]
		oomAdj, err := strconv.Atoi(oomAdjString)
		if err != nil {
			testing.ContextLog(ctx, "Warning: ArcMemoryKillObserver failed to parse oom_adj: ", err)
			return
		}
		if oomAdj >= 900 { // ProcessList.CACHED_APP_MIN_ADJ
			recordLocation = &o.Cached
		} else if oomAdj > 0 { // ProcessList.FOREGROUND_APP_ADJ
			recordLocation = &o.Perceptible
		} else {
			recordLocation = &o.Foreground
		}
		testing.ContextLogf(ctx, "LMKD kill observed, %q, %d", packageString, oomAdj)
	} else {
		return
	}

	if *recordLocation == nil {
		t, err := adb.ParseLogcatTimestamp(timeString)
		if err != nil {
			testing.ContextLog(ctx, "Warning: ArcMemoryKillObserver failed to parse timestamp: ", err)
			return
		}
		*recordLocation = &ArcMemoryKillInfo{t, packageString}
	}
}

// NewArcMemoryKillObserver creates a new ArcMemoryKillObserver.
func NewArcMemoryKillObserver(ctx context.Context, a *arc.ARC) (*ArcMemoryKillObserver, error) {
	now, err := a.LogcatDeviceTime(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logcat device time")
	}
	observeContext, cancel := context.WithCancel(ctx)
	o := &ArcMemoryKillObserver{cancel, nil, nil, nil, nil}
	go func() {
		o.Error = a.WaitForLogcatSince(observeContext, func(line string) bool {
			o.observeLine(observeContext, line)
			return false
		}, now)
	}()
	return o, nil
}
