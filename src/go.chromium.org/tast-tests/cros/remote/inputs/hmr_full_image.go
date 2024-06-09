// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

// FullImageResult contains the output of the HMR full image sliding analysis.
type FullImageResult struct {
	Passed      bool
	TotalMSE    float64 // total mean squared error of all paths.
	Epsilon     float64 // maximum distance between start points on reference and result paths in mm.
	PathResults []*PathResult
}

// PathResult contains the output of the HMR full image sliding analysis over a single path.
type PathResult struct {
	PathMSE              float64 // average mean squared error for all calculations on path.
	SuccessfulFitRate    float64 // percentage of curve fittings performed with an r2 value greater than the threshold.
	NumberOfCalculations float64 // number of curve fitting calculations performed.
	ReferencePathLength  int
	ResultPathLength     int
	BubbleRadius         float64 // value in (mm) used in the sliding window analysis for this path.
}
