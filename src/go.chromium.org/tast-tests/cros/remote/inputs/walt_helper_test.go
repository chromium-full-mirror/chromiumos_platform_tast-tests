// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"fmt"
	"testing"

	"go.chromium.org/tast/core/errors"
)

const (
	minimizationWaltOutput = `Starting drag latency test
Input device   : /dev/input/event4
Serial device  : /dev/ttyACM1
Laser log file : /tmp/WALT_2017_03_07__1532_12_laser.log
evtest log file: /tmp/WALT_2017_03_07__1532_12_evtest.log
Clock zeroed at 1488918733 (rt 0.306 ms)
...
Processing data, may take a minute or two...
Drag latency (min method) = 21.07 ms
`
	defaultWaltOutput = `Starting drag latency test
Input device   : /dev/input/event4
Serial device  : /dev/ttyACM1
Laser log file : /tmp/WALT_2017_03_07__1532_12_laser.log
evtest log file: /tmp/WALT_2017_03_07__1532_12_evtest.log
Clock zeroed at 1488918733 (rt 0.306 ms)
...
Average Maximum Minimum
0.0237723313845 0.0333168506622 0.0167829990387
Latency (average): ... ms
`
	invalidWaltOutput = `...
0.0237723313845 0.0333168506622 0.0167829990387
Latency (average): ... ms
`
)

// TestParseMetricLine tests an expected successful outcome for parseMetricLine.
func TestParseMetricLine(t *testing.T) {
	var trueAvg, trueMax, trueMin = 9.045, 10.001, 8.032
	// The metric line is reported in seconds, so convert the true latencies to seconds.
	var line = fmt.Sprintf("%f %f %f", trueAvg/1000, trueMax/1000, trueMin/1000)

	testAvg, testMax, testMin, err := parseMetricLine(line)
	if err != nil {
		t.Fatalf("Failed to parse metric line: %v", err)
	}

	if testAvg != trueAvg {
		t.Errorf("Incorrect value parsed for average: %f, should be: %f", testAvg, trueAvg)
	}
	if testMax != trueMax {
		t.Errorf("Incorrect value parsed for maximum: %f, should be: %f", testMax, trueMax)
	}
	if testMin != trueMin {
		t.Errorf("Incorrect value parsed for minimum: %f, should be: %f", testMin, trueMin)
	}
}

func TestParseMinimizationLine(t *testing.T) {
	var trueLatency, trueMax, trueMin = 9.045, 0.0, 0.0
	var line = fmt.Sprintf("Drag latency (min method) = %f ms", trueLatency)

	testLatency, testMax, testMin, err := parseMinimizationLine(line)

	if err != nil {
		t.Fatal(err)
	}

	if testLatency != trueLatency {
		t.Errorf("Incorrect value parsed for average: %f, should be: %f", testLatency, trueLatency)
	}
	if testMax != trueMax {
		t.Errorf("Incorrect value parsed for maximum: %f, should be: %f", testMax, trueMax)
	}
	if testMin != trueMin {
		t.Errorf("Incorrect value parsed for minimum: %f, should be: %f", testMin, trueMin)
	}
}

func TestMinimizationParseLatency(t *testing.T) {
	var trueLatency, trueMax, trueMin = 21.07, 0.0, 0.0

	testLatency, testMax, testMin, err := ParseWaltLatency(minimizationWaltOutput)
	if err != nil {
		t.Fatalf("Failed to parse minimization latency: %v", err)
	}

	if testLatency != trueLatency {
		t.Errorf("Incorrect value parsed for latency: %f, should be: %f", testLatency, trueLatency)
	}
	if testMax != trueMax {
		t.Errorf("Incorrect value parsed for maximum: %f, should be: %f", testMax, trueMax)
	}
	if testMin != trueMin {
		t.Errorf("Incorrect value parsed for minimum: %f, should be: %f", testMin, trueMin)
	}
}

func TestParseLatency(t *testing.T) {
	// Latencies are in milliseconds, but in the defaultWaltOutput they are in seconds.
	var trueAvg, trueMax, trueMin = 23.7723313845, 33.3168506622, 16.7829990387

	testAvg, testMax, testMin, err := ParseWaltLatency(defaultWaltOutput)
	if err != nil {
		t.Fatalf("Failed to parse latency: %v", err)
	}

	if testAvg != trueAvg {
		t.Errorf("Incorrect value parsed for average: %f, should be: %f", testAvg, trueAvg)
	}
	if testMax != trueMax {
		t.Errorf("Incorrect value parsed for maximum: %f, should be: %f", testMax, trueMax)
	}
	if testMin != trueMin {
		t.Errorf("Incorrect value parsed for minimum: %f, should be: %f", testMin, trueMin)
	}
}

func TestErrOutputFormatParseLatency(t *testing.T) {
	_, _, _, err := ParseWaltLatency(invalidWaltOutput)

	if !errors.Is(err, errOutputFormat) {
		t.Fatalf("Expected error errOutputFormat, got: %v", err)
	}
}
