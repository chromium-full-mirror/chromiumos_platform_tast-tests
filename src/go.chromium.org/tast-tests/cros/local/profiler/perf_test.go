// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package profiler

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.chromium.org/tast/core/testutil"
)

func TestParseStatFile(t *testing.T) {
	// The test data comes from command like
	// perf stat -a -p 8113 -e cycles --output perf_stat_only.data
	const data = `# started on Tue Aug 11 17:50:16 2020


	 Performance counter stats for process id '8113':

	          190435360      cycles

		         7.999881391 seconds time elapsed

	`

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "perf_stat.data")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal("Failed to create perf_stat.data: ", err)
	}

	cyclesPerSecond, err := parseStatFileCycles(path)
	if err != nil {
		t.Fatal("Failed to parse stat file: ", err)
	}

	expected := 23804772.932539094

	if math.Abs(expected-cyclesPerSecond[0].Value) >= 1e-6 {
		t.Errorf("Unexpected cycles per second: got %#v; want %#v", cyclesPerSecond, expected)
	}
}

func TestParseStatFileECore(t *testing.T) {
	// The test data comes from command like
	// perf stat -a -p 8113 -e cycles --output perf_stat_only.data
	const data = `
	Performance counter stats for 'system wide':

		   9645.94 msec cpu-clock                 #   11.996 CPUs utilized
		       423      context-switches          #   43.853 /sec
		        24      cpu-migrations            #    2.488 /sec
		         4      page-faults               #    0.415 /sec
		  19307305      cpu_core/cycles/          #    2.002 M/sec
		  17364020      cpu_atom/cycles/          #    1.800 M/sec
		  12811040      cpu_core/instructions/    #    1.328 M/sec
		   6279887      cpu_atom/instructions/    #  651.039 K/sec
		   2390023      cpu_core/branches/        #  247.775 K/sec
		   1188111      cpu_atom/branches/        #  123.172 K/sec
		    113804      cpu_core/branch-misses/   #   11.798 K/sec
		    132679      cpu_atom/branch-misses/   #   13.755 K/sec

       0.804111502 seconds time elapsed
	`

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "perf_stat.data")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal("Failed to create perf_stat.data: ", err)
	}

	cyclesPerSecond, err := parseStatFileCycles(path)
	if err != nil {
		t.Fatal("Failed to parse stat file: ", err)
	}

	expected := []float64{2.40107310391389e+07, 2.159404505073228e+07}

	if len(cyclesPerSecond) != 2 {
		t.Errorf("Unexpected number of cycles per second values: got %d; want %d", len(cyclesPerSecond), 2)
	}
	if math.Abs(expected[0]-cyclesPerSecond[0].Value) >= 1e-6 {
		t.Errorf("Unexpected cycles per second: got %#v; want %#v", cyclesPerSecond[0].Value, expected[0])
	}
	if math.Abs(expected[1]-cyclesPerSecond[1].Value) >= 1e-6 {
		t.Errorf("Unexpected cycles per second: got %#v; want %#v", cyclesPerSecond[1].Value, expected[1])
	}
}

func TestParseStatFileNoCycle(t *testing.T) {
	// The test data comes from command like
	// perf stat -a -p 8113 -e cycles --output perf_stat_only.data
	const data = `# started on Tue Aug 11 17:50:16 2020


	 Performance counter stats for process id '8113':

	          <not counted>      cycles

		         7.999881391 seconds time elapsed

	`

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "perf_stat_no_cycle.data")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal("Failed to create perf_stat_no_cycle.data: ", err)
	}

	_, err := parseStatFileCycles(path)
	if err == nil || !strings.Contains(err.Error(), "got 0 cycle") {
		t.Fatal("Failed to check stat file with no cycle: ", err)
	}
}

func TestParseStatInstructionsAtIntervals(t *testing.T) {
	// The test data comes from command like
	// perf stat -a -e instructions -I 500 --output perf_stat_instructions_at_intervals.data
	const data = `#           time             counts unit events
     0.500642977          356311273      instructions
     1.001999299          376048978      instructions
	`

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "perf_stat_instructions_at_intervals.data")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal("Failed to create perf_stat_instructions_at_intervals.data: ", err)
	}

	timestampedData, err := parseStatFileInstructions(path)
	if err != nil {
		t.Fatal("Failed to parse stat file: ", err)
	}

	expected := make([]valueWithTimestamp, 2)
	expected[0].Timestamp, _ = time.ParseDuration("0.500642977s")
	expected[0].Value = 356311273
	expected[1].Timestamp, _ = time.ParseDuration("1.001999299s")
	expected[1].Value = 376048978

	if len(timestampedData) != len(expected) {
		t.Errorf("Unexpected number of timestamped values: got %d; want %d", len(timestampedData), len(expected))
	}
	for i, expectedData := range expected {
		if expectedData != timestampedData[i] {
			t.Errorf("Unexpected data at index %d: got %v, want %v", i, timestampedData[i], expectedData)
		}
	}
}
