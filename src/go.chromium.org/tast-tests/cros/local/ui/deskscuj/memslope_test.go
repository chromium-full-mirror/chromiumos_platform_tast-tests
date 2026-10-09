// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"math"
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/perf"
)

func TestLinearSlope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		x, y    []float64
		want    float64
		wantErr bool
	}{
		{
			name: "exact line",
			x:    []float64{5, 9, 11, 12, 16},
			y:    []float64{110, 190, 230, 250, 330},
			want: 20,
		},
		{
			name: "decreasing",
			x:    []float64{1, 2, 3},
			y:    []float64{30, 20, 10},
			want: -10,
		},
		{
			name: "noisy",
			x:    []float64{1, 2, 3, 4},
			y:    []float64{1, 3, 2, 4},
			want: 0.8,
		},
		{
			name: "flat",
			x:    []float64{1, 2, 3},
			y:    []float64{7, 7, 7},
			want: 0,
		},
		{
			name:    "one point",
			x:       []float64{1},
			y:       []float64{1},
			wantErr: true,
		},
		{
			name:    "equal x",
			x:       []float64{2, 2},
			y:       []float64{1, 3},
			wantErr: true,
		},
		{
			name:    "mismatched lengths",
			x:       []float64{1, 2},
			y:       []float64{1},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := linearSlope(tc.x, tc.y)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("linearSlope(%v, %v) = %v; want an error", tc.x, tc.y, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("linearSlope(%v, %v) failed: %v", tc.x, tc.y, err)
			}
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("linearSlope(%v, %v) = %v; want %v", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

// recordValues runs tr.record and returns the reported values by name.
func recordValues(tr *memTracker) map[string][]float64 {
	pv := perf.NewValues()
	tr.record(context.Background(), pv)
	got := make(map[string][]float64)
	for m, vs := range pv.GetValues() {
		got[m.Name] = vs
	}
	return got
}

// checkValues checks that each named metric has exactly the wanted value.
func checkValues(t *testing.T, got map[string][]float64, want map[string]float64) {
	t.Helper()
	for name, w := range want {
		if vs := got[name]; len(vs) != 1 || math.Abs(vs[0]-w) > 1e-9 {
			t.Errorf("%s = %v; want [%v]", name, vs, w)
		}
	}
}

func TestMemTrackerRecord(t *testing.T) {
	tr := newMemTracker("Test")
	for _, w := range []int{5, 9, 12} {
		f := float64(w)
		tr.samples = append(tr.samples, memSample{
			windows:            w,
			systemMB:           1000 + 50*f,
			chromeMB:           500 + 30*f,
			gpuMB:              100 + 2*f,
			rendererMB:         300 + 20*f,
			chromeWithSwapMB:   500 + 40*f,
			gpuWithSwapMB:      100 + 3*f,
			rendererWithSwapMB: 300 + 25*f,
		})
	}
	got := recordValues(tr)
	checkValues(t, got, map[string]float64{
		"Test.MemSlope.SystemUsed":        50,
		"Test.MemSlope.ChromePSS":         30,
		"Test.MemSlope.ChromePSSWithSwap": 40,
		"Test.MemSlope.GPU":               2,
		"Test.MemSlope.GPUWithSwap":       3,
		"Test.MemSlope.Renderer":          20,
		"Test.MemSlope.RendererWithSwap":  25,
	})
	if vs := got["Test.Mem.Windows"]; len(vs) != 3 || vs[0] != 5 || vs[2] != 12 {
		t.Errorf("Test.Mem.Windows = %v; want [5 9 12]", vs)
	}
	if vs := got["Test.Mem.SystemUsed"]; len(vs) != 3 {
		t.Errorf("Test.Mem.SystemUsed = %v; want 3 values", vs)
	}
	// Without the shown and final snapshots, their values aren't reported.
	for name := range got {
		for _, prefix := range []string{"Test.MemPerShownWindow.", "Test.Mem.Final.", "Test.MemGrowth."} {
			if strings.HasPrefix(name, prefix) {
				t.Errorf("%s reported without the snapshot it needs", name)
			}
		}
	}
}

func TestMemTrackerRecordShownAndFinal(t *testing.T) {
	tr := newMemTracker("Test")
	tr.samples = []memSample{
		{windows: 5, systemMB: 1000, chromeMB: 500},
		{windows: 12, systemMB: 1350, chromeMB: 710},
	}
	// 4 more windows, whose popups were shown, cost more per window. They
	// must not change the slope.
	tr.shown = &memSample{windows: 16, systemMB: 1590, chromeMB: 870}
	// The final snapshot is taken with the same number of windows, after
	// the workload.
	tr.final = &memSample{windows: 16, systemMB: 1900, chromeMB: 1000}

	got := recordValues(tr)
	checkValues(t, got, map[string]float64{
		"Test.MemSlope.SystemUsed":          50,
		"Test.MemSlope.ChromePSS":           30,
		"Test.MemPerShownWindow.SystemUsed": 60,
		"Test.MemPerShownWindow.ChromePSS":  40,
		"Test.Mem.Final.SystemUsed":         1900,
		"Test.Mem.Final.ChromePSS":          1000,
		"Test.MemGrowth.SystemUsed":         310,
		"Test.MemGrowth.ChromePSS":          130,
	})
	if vs := got["Test.Mem.Windows"]; len(vs) != 2 {
		t.Errorf("Test.Mem.Windows = %v; want the 2 slope snapshots", vs)
	}
}

func TestMemTrackerRecordFinalWithoutShown(t *testing.T) {
	tr := newMemTracker("Test")
	tr.samples = []memSample{
		{windows: 5, systemMB: 1000},
		{windows: 12, systemMB: 1350},
	}
	tr.final = &memSample{windows: 12, systemMB: 1500}

	got := recordValues(tr)
	// The growth is since the last slope snapshot.
	checkValues(t, got, map[string]float64{
		"Test.MemSlope.SystemUsed":  50,
		"Test.Mem.Final.SystemUsed": 1500,
		"Test.MemGrowth.SystemUsed": 150,
	})
}
