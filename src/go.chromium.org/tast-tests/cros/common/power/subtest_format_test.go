// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"testing"
	"time"
)

// measureStarted < measureEnded < tsData[0]
func TestNoOverlapBeforeTsDataReturnsErr(t *testing.T) {
	measureStarted := time.Unix(0, 0)
	measureEnded := time.Unix(1, 0)
	tsData := []time.Time{time.Unix(3, 0), time.Unix(4, 0), time.Unix(5, 0)}

	_, _, err := GetOverlapIndices(measureStarted, measureEnded, tsData)
	if err == nil {
		t.Fatal("Test should return error when subtest finishes before timestamp data")
	}
}

// measureStarted < measureEnded = tsData[0]
func TestNoOverlapBeforeOrEqualToTsDataReturnsErr(t *testing.T) {
	measureStarted := time.Unix(0, 0)
	measureEnded := time.Unix(3, 0)
	tsData := []time.Time{time.Unix(3, 0), time.Unix(4, 0), time.Unix(5, 0)}

	_, _, err := GetOverlapIndices(measureStarted, measureEnded, tsData)
	if err == nil {
		t.Fatal("Test should return error when subtest finishes at first timestamp data")
	}
}

// tsData[0] = measureStarted < measureEnded = tsData[len(tsData)-1]
func TestOverlapAtStartAndEnd(t *testing.T) {
	tsData := []time.Time{time.Unix(1, 0), time.Unix(2, 0), time.Unix(3, 0), time.Unix(4, 0), time.Unix(5, 0)}
	measureStarted := time.Unix(1, 0)
	measureEnded := time.Unix(5, 0)

	startIdx, endIdx := 0, 4

	s, e, err := GetOverlapIndices(measureStarted, measureEnded, tsData)
	if err != nil {
		t.Fatal("Failed to get overlap indices: ", err)
	}
	if !(startIdx == s && endIdx == e) {
		t.Fatal("Failed to calculate correct overlap indices")
	}
}

// tsData[0] < measureStarted < measureEnded < tsData[len(tsData)-1]
func TestOverlapBetweenStartAndEnd(t *testing.T) {
	tsData := []time.Time{time.Unix(1, 0), time.Unix(2, 0), time.Unix(3, 0), time.Unix(4, 0), time.Unix(5, 0)}
	measureStarted := time.Unix(2, 0)
	measureEnded := time.Unix(4, 0)

	startIdx, endIdx := 1, 3

	s, e, err := GetOverlapIndices(measureStarted, measureEnded, tsData)
	if err != nil {
		t.Fatal("Failed to get overlap indices: ", err)
	}
	if !(startIdx == s && endIdx == e) {
		t.Fatal("Failed to calculate correct overlap indices")
	}
}

// tsData[len(tsData)-1] = measureStarted < measureEnded
func TestNoOverlapAfterOrEqualToTsDataReturnsErr(t *testing.T) {
	tsData := []time.Time{time.Unix(1, 0), time.Unix(2, 0), time.Unix(3, 0), time.Unix(4, 0), time.Unix(5, 0)}
	measureStarted := time.Unix(5, 0)
	measureEnded := time.Unix(7, 0)

	_, _, err := GetOverlapIndices(measureStarted, measureEnded, tsData)
	if err == nil {
		t.Fatal("Test should return error when subtest starts at last timestamp data")
	}

}

// tsData[len(tsData)-1] < measureStarted < measureEnded
func TestNoOverlapAfterTsDataReturnsErr(t *testing.T) {
	tsData := []time.Time{time.Unix(1, 0), time.Unix(2, 0), time.Unix(3, 0), time.Unix(4, 0), time.Unix(5, 0)}
	measureStarted := time.Unix(6, 0)
	measureEnded := time.Unix(7, 0)

	_, _, err := GetOverlapIndices(measureStarted, measureEnded, tsData)
	if err == nil {
		t.Fatal("Test should return error when subtest finishes after last timestamp data")
	}

}
