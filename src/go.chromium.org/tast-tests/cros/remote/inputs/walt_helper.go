// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"bufio"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/errors"
)

const (
	metricFormatLine         = "Average Maximum Minimum"
	minimizationMetricFormat = "Drag latency (min method) = "

	// TODO(b/358616722): Update WALT to include max and min values when using
	// the minimization method.
	// Minimization output does not contain max and min values; set defaults here.
	minimizationMax = 0.0
	minimizationMin = 0.0

	// DefaultWaltSerialPort is the default serial port file used by WALT.
	DefaultWaltSerialPort = "/dev/ttyACM0"
)

// StylusLatencyResult is the result of a stylus latency regression test.
type StylusLatencyResult struct {
	Passed     bool
	AvgLatency float64
	MaxLatency float64
	MinLatency float64
}

var errOutputFormat = errors.New("WALT output incorrectly formatted")

// ParseWaltLatency parses the output string of the WALT command for the latency calculation.
func ParseWaltLatency(output string) (float64, float64, float64, error) {
	// If the metric line is found, this error will be overwritten.
	var formatErr error = errOutputFormat
	var average, max, min float64

	scanner := bufio.NewScanner(strings.NewReader(output))
	/*
		The WALT default output is formatted as:
		...
		Average Maximum Minimum
		{Avg Value} {Max Value} {Min Value}
		...

		The WALT minimization output is formatted as:
		...
		Drag latency (min method) = {Value} ms
	*/
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), metricFormatLine) {
			// Default method was used for the latency calculation.
			if scanner.Scan() { // Advance by one line in the output.
				average, max, min, formatErr = parseMetricLine(scanner.Text())
			}
			break
		} else if strings.Contains(scanner.Text(), minimizationMetricFormat) {
			// Minimization method was used for the latency calculation.
			average, max, min, formatErr = parseMinimizationLine(scanner.Text())
			break
		}
	}

	// Check for errors from parsing the output.
	if scanner.Err() != nil || formatErr != nil {
		err := errors.Join(formatErr, scanner.Err())
		return 0, 0, 0, err
	}

	return average, max, min, nil
}

// parseMetricLine parses the metric line from the default latency calculations
// and returns the latency values (in milliseconds).
func parseMetricLine(line string) (float64, float64, float64, error) {
	var averageSec, maxSec, minSec float64

	_, err := fmt.Sscanf(line, "%f %f %f", &averageSec, &maxSec, &minSec)
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed to parse metrics")
	}

	// Convert latency values to milliseconds.
	return averageSec * 1000, maxSec * 1000, minSec * 1000, nil
}

// parseMinimizationLine parses the metric line from the minimization latency
// calculations and returns the latency values (in milliseconds).
func parseMinimizationLine(line string) (float64, float64, float64, error) {
	var latency float64

	_, err := fmt.Sscanf(line, minimizationMetricFormat+" %f ms", &latency)
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed to parse minimization metrics")
	}
	return latency, minimizationMax, minimizationMin, err
}

// SaveLatencyMetrics saves the latency metrics measured by WALT for Crosbolt.
func SaveLatencyMetrics(latencyResult *StylusLatencyResult, savePath string) error {
	pv := perf.NewValues()

	pv.Set(perf.Metric{
		Name:      "avg_latency",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, latencyResult.AvgLatency)
	pv.Set(perf.Metric{
		Name:      "max_latency",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, latencyResult.MaxLatency)
	pv.Set(perf.Metric{
		Name:      "min_latency",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, latencyResult.MinLatency)

	if err := pv.Save(savePath); err != nil {
		return errors.Wrap(err, "failed to save latency metrics for crosbolt")
	}

	return nil
}
