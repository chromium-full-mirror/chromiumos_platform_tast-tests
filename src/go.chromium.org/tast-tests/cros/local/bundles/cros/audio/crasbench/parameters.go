// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crasbench

// Parameters contains all the data needed to run a single test iteration.
type Parameters struct {
	BenchmarkFilter  string // cras_bench filter for this test case.
	MetricFps        bool   // If the result contains metric FPS.
	Time4096Frame    bool   // If the result contains metric "time_per_4096_frames".
	MaxTime4096Frame bool   // If the result contains metric "max_time_per_4096_frames".
	DLC              string // DLC dependencies to install before running the test.
}

// For testing.Test.Params
var (
	APM = Parameters{
		BenchmarkFilter:  "BM_Apm",
		MetricFps:        true,
		Time4096Frame:    false,
		MaxTime4096Frame: false,
	}
	DSP = Parameters{
		BenchmarkFilter:  "BM_Dsp",
		MetricFps:        true,
		Time4096Frame:    false,
		MaxTime4096Frame: false,
	}
	AM = Parameters{ // Audio Model.
		BenchmarkFilter:  "BM_Am",
		MetricFps:        false,
		Time4096Frame:    false,
		MaxTime4096Frame: false,
		DLC:              "sr-bt-dlc",
	}
	MixerOps = Parameters{
		BenchmarkFilter:  "BM_CrasMixerOps",
		MetricFps:        false,
		Time4096Frame:    false,
		MaxTime4096Frame: false,
	}
	Alsa = Parameters{
		BenchmarkFilter:  "BM_Alsa",
		MetricFps:        true,
		Time4096Frame:    true,
		MaxTime4096Frame: true,
	}
)
