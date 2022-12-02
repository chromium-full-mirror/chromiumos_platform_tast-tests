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

	"chromiumos/tast/errors"
	"chromiumos/tast/fsutil"
	"chromiumos/tast/testing"
)

const (
	tracingPath = "/sys/kernel/tracing"
)

// ParseFPSTrace parses the trace file in tracePath.
func ParseFPSTrace(tracePath string) ([][]float64, error) {
	// Line format:
	// <proc> [000] d.h1 87154.652132: drm_vblank_event: crtc=0, seq=49720
	// TODO(b/172225622): Do we need to care about seq?
	re := regexp.MustCompile(`^.* ([0-9\.]+): drm_vblank_event: crtc=(\d+).*$`)

	trace, err := os.Open(tracePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open trace file")
	}
	defer trace.Close()

	var data [][]float64
	lastEvent := 0.0
	scanner := bufio.NewScanner(trace)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := re.FindStringSubmatch(line); matches != nil {
			matchedCrtc, err := strconv.Atoi(matches[2])
			if err != nil {
				return nil, errors.Wrap(err, "error converting crtc to int")
			}

			for len(data) <= matchedCrtc {
				data = append(data, make([]float64, 0))
			}

			event, err := strconv.ParseFloat(matches[1], 64)
			if err != nil {
				return nil, errors.Wrap(err, "error converting time to float")
			}
			if lastEvent != 0.0 {
				data[matchedCrtc] = append(data[matchedCrtc], 1.0/(event-lastEvent))
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
	tracePath := filepath.Join(tracingPath, "trace")
	if err := ioutil.WriteFile(tracePath, nil, 0644); err != nil {
		return errors.Wrap(err, "cannot clear trace buffer")
	}
	return nil
}

// CollectFPSTrace uses tracefs to collect FPS data over a duration, and save the resulting trace file for later analysis.
func CollectFPSTrace(ctx context.Context, collectTime time.Duration, outputPath string) error {
	// Enable vblank events.
	vblankPath := filepath.Join(tracingPath,
		"events/drm/drm_vblank_event/enable")
	if err := ioutil.WriteFile(vblankPath, []byte("1"), 0644); err != nil {
		return errors.Wrap(err, "cannot enable drm vblank event tracing")
	}
	defer ioutil.WriteFile(vblankPath, []byte("0"), 0644)

	// Collect trace data.
	tracingOnPath := filepath.Join(tracingPath, "tracing_on")
	if err := ioutil.WriteFile(tracingOnPath, []byte("1"), 0644); err != nil {
		return errors.Wrap(err, "cannot enable tracing")
	}
	defer ioutil.WriteFile(tracingOnPath, []byte("0"), 0644)

	testing.ContextLog(ctx, "Collecting vblank event samples")
	if err := testing.Sleep(ctx, collectTime); err != nil {
		return errors.Wrap(err, "cannot sleep")
	}

	ioutil.WriteFile(tracingOnPath, []byte("0"), 0644)

	tracePath := filepath.Join(tracingPath, "trace")

	// Save trace file in output directory.
	if err := fsutil.CopyFile(tracePath, outputPath); err != nil {
		return errors.Wrap(err, "failed to copy trace file")
	}
	return nil
}

// FPSStats holds statistics to describe FPS calculation.
type FPSStats struct {
	Mean       float64
	Stddev     float64
	Min        float64
	Max        float64
	NumSamples int
}

// CalculateFPSStats calculates FPS statistics from the provided data.
func CalculateFPSStats(data []float64, trimPercent int) FPSStats {
	// Sort and trim a copy of the data.
	data = data[:]
	sort.Float64s(data)
	trim := len(data) * trimPercent / 100
	data = data[trim : len(data)-trim]

	var sum float64
	var sum2 float64
	for _, x := range data {
		sum += x
		sum2 += x * x
	}
	n := float64(len(data))
	mean := sum / n
	stddev := math.Sqrt((sum2 / n) - (mean * mean))

	return FPSStats{mean, stddev, data[0], data[len(data)-1], len(data)}
}
