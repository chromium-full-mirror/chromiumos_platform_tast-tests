// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"bufio"
	"context"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	tracingPath = "/sys/kernel/tracing"
)

var (
	vblankEventEnable = filepath.Join(tracingPath,
		"events/drm/drm_vblank_event/enable")
	tracingOn = filepath.Join(tracingPath, "tracing_on")
	traceFile = filepath.Join(tracingPath, "trace")
)

// ParseVblankTrace parses the trace file at tracePath into an array of frame timing deltas per each CRTC.
func ParseVblankTrace(tracePath string) ([][]time.Duration, error) {
	// Line format:
	// <proc> [000] d.h1 87154.652132: drm_vblank_event: crtc=0, seq=49720
	// TODO(b/172225622): Do we need to care about seq?
	re := regexp.MustCompile(`^.* ([0-9\.]+): drm_vblank_event: crtc=(\d+).*$`)

	trace, err := os.Open(tracePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open trace file")
	}
	defer trace.Close()

	var data [][]time.Duration
	var lastEvent time.Duration = 0
	scanner := bufio.NewScanner(trace)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := re.FindStringSubmatch(line); matches != nil {
			matchedCrtc, err := strconv.Atoi(matches[2])
			if err != nil {
				return nil, errors.Wrap(err, "error converting crtc to int")
			}

			for len(data) <= matchedCrtc {
				data = append(data, make([]time.Duration, 0))
			}

			event, err := time.ParseDuration(matches[1] + "s")
			if err != nil {
				return nil, errors.Wrap(err, "error parsing vblank event time")
			}
			if lastEvent != 0 {
				data[matchedCrtc] = append(data[matchedCrtc], event-lastEvent)
			}
			lastEvent = event
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "error reading trace file")
	}

	if len(data) == 0 {
		return nil, errors.New("no data in trace file")
	}

	return data, nil
}

// ClearTraceBuffer clears the tracefs buffer.
func ClearTraceBuffer() error {
	if err := ioutil.WriteFile(traceFile, nil, 0644); err != nil {
		return errors.Wrap(err, "cannot clear trace buffer")
	}
	return nil
}

// EnableVblankTrace enables vblank event tracing and turns on tracing.
func EnableVblankTrace(ctx context.Context) error {
	// Enable vblank event tracing.
	if err := ioutil.WriteFile(vblankEventEnable, []byte("1"), 0644); err != nil {
		return errors.Wrap(err, "cannot enable drm vblank event tracing")
	}

	// Turn on tracing.
	if err := ioutil.WriteFile(tracingOn, []byte("1"), 0644); err != nil {
		return errors.Wrap(err, "cannot enable tracing")
	}
	return nil
}

// DisableVblankTrace disables vblank event tracing and turns off tracing.
func DisableVblankTrace() {
	ioutil.WriteFile(vblankEventEnable, []byte("0"), 0644)
	ioutil.WriteFile(tracingOn, []byte("0"), 0644)
}

// CopyVblankTrace copies the tracefs buffer to the specified output location.
func CopyVblankTrace(ctx context.Context, outputPath string) error {
	if err := fsutil.CopyFile(traceFile, outputPath); err != nil {
		return errors.Wrap(err, "failed to copy trace file")
	}
	return nil
}

// CollectVblankTrace uses tracefs to collect vblank event data over a duration, and save the resulting trace file for later analysis.
func CollectVblankTrace(ctx context.Context, collectTime time.Duration, outputPath string) error {
	if err := EnableVblankTrace(ctx); err != nil {
		return errors.Wrap(err, "failed to enabled vblank tracing")
	}
	defer DisableVblankTrace()

	testing.ContextLog(ctx, "Collecting vblank event samples")
	// GoBigSleepLint: Wait for tracing samples.
	if err := testing.Sleep(ctx, collectTime); err != nil {
		return errors.Wrap(err, "cannot sleep")
	}

	DisableVblankTrace()
	if err := CopyVblankTrace(ctx, outputPath); err != nil {
		return err
	}
	return nil
}

// VblankStats holds statistics about vblank tracing data.
type VblankStats struct {
	Mean       time.Duration
	Stddev     time.Duration
	Min        time.Duration
	Max        time.Duration
	NumSamples int
}

// CalculateVblankStats calculates refresh interval statistics from the provided vblank event data.
func CalculateVblankStats(data []time.Duration, trimPercent int) VblankStats {
	// Sort and trim a copy of the data.
	data = data[:]
	sort.Slice(data, func(i, j int) bool { return data[i] > data[j] })
	trim := len(data) * trimPercent / 100
	data = data[trim : len(data)-trim]

	var sum time.Duration
	var sum2 time.Duration
	for _, x := range data {
		sum += x
		sum2 += x * x
	}
	n := time.Duration(len(data))
	mean := sum / n
	stddev := time.Duration(math.Sqrt(float64((sum2 / n) - (mean * mean))))

	return VblankStats{mean, stddev, data[0], data[len(data)-1], len(data)}
}

// MsF converts a time.Duration into a floating point millisecond representation. This is separate
// from time.Duration.Milliseconds which returns an integer value.
func MsF(t time.Duration) float64 {
	return float64(t) / float64(time.Millisecond)
}

// Hz converts a time.Duration into Hertz.
func Hz(t time.Duration) float64 {
	return 1.0 / time.Duration.Seconds(t)
}

// FromHz converts a Hertz value into a time.Duration.
func FromHz(f float64) time.Duration {
	return time.Duration(float64(time.Second) / f)
}
