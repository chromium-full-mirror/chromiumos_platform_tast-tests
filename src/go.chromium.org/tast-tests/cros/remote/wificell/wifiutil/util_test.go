// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"testing"
)

func TestAverageFloat(t *testing.T) {
	floats := []float64{0.5, 0.5, 1.0, 1.0}
	var expectedAverage float64 = 0.75
	actualAverage := Average(floats)
	if actualAverage != expectedAverage {
		t.Fatalf("Average(%v) = %f; want %f", floats, actualAverage, expectedAverage)
	}
}

func TestAverageInt(t *testing.T) {
	ints := []int64{1, 1, 2, 2}
	var expectedAverage float64 = 1.5
	actualAverage := Average(ints)
	if actualAverage != expectedAverage {
		t.Fatalf("Average(%v) = %f; want %f", ints, actualAverage, expectedAverage)
	}
}

func TestAverageEmpty(t *testing.T) {
	var empty []int64 = []int64{}
	var expectedAverage float64 = 0
	actualAverage := Average(empty)
	if actualAverage != expectedAverage {
		t.Fatalf("Average(%v) = %f; want %f", empty, actualAverage, expectedAverage)
	}
}
